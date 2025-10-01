// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rustyoz/svg"
)

var (
	lfxEnvironment string
)

// ProjectBase represents a project from NATS KV store
type ProjectBase struct {
	UID                        string     `json:"uid"`
	Slug                       string     `json:"slug"`
	Name                       string     `json:"name"`
	Description                string     `json:"description"`
	Public                     bool       `json:"public"`
	ParentUID                  string     `json:"parent_uid"`
	Stage                      string     `json:"stage"`
	Category                   string     `json:"category"`
	LegalEntityType            string     `json:"legal_entity_type"`
	LegalEntityName            string     `json:"legal_entity_name"`
	LegalParentUID             string     `json:"legal_parent_uid"`
	FundingModel               []string   `json:"funding_model"`
	EntityDissolutionDate      *time.Time `json:"entity_dissolution_date"`
	EntityFormationDocumentURL string     `json:"entity_formation_document_url"`
	FormationDate              *time.Time `json:"formation_date"`
	AutojoinEnabled            bool       `json:"autojoin_enabled"`
	CharterURL                 string     `json:"charter_url"`
	LogoURL                    string     `json:"logo_url"`
	WebsiteURL                 string     `json:"website_url"`
	RepositoryURL              string     `json:"repository_url"`
	CreatedAt                  *time.Time `json:"created_at"`
	UpdatedAt                  *time.Time `json:"updated_at"`
}

const (
	defaultHeight = 800
	defaultWidth  = 1600
)

// createFile creates a file for a file path
func createFile(filePath string) (*os.File, error) {
	out, err := os.Create(filePath)
	if err != nil {
		slog.Error("error creating file", "error", err)
		return nil, err
	}

	return out, err
}

type ImageDimensions struct {
	Width  int
	Height int
}

// downloadFile tries to download an image file from a url into a local file, and then returns
// the image dimensions by parsing the original .svg file
func downloadFile(url string, out *os.File) (imgDimensions *ImageDimensions, err error) {
	downloadImageTime := time.Now()
	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		slog.Error("http bad status", "url", url, "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Check server response status code before trying to read the response body
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status %d while downloading %s", resp.StatusCode, url)
		slog.Error("http bad status", "url", url, "status_code", resp.StatusCode)
		return nil, err
	}

	// Duplicate io.ReadCloser so it can be used for writing to the file and for parsing the svg file content
	var buf bytes.Buffer
	respBody := io.TeeReader(resp.Body, &buf)

	// Recover from a panic that can be caused by svg.ParseSvgFromReader, which we don't have control over
	defer func() {
		if r := recover(); r != nil {
			// Write the body to file
			_, err = io.Copy(out, &buf)
			if err != nil {
				slog.Error("error copying response body", "url", url, "error", err)
				return
			}

			// Use default width and height when image dimensions can't be retrieved
			imgWidth := defaultWidth
			imgHeight := defaultHeight
			slog.Info("image download time",
				"url", url,
				"image_width", imgWidth,
				"image_height", imgHeight,
				"duration", time.Since(downloadImageTime).String())
			imgDimensions = &ImageDimensions{Width: imgWidth, Height: imgHeight}
			err = nil
		}

	}()

	// Get the file (image) dimensions
	svgImg, err := svg.ParseSvgFromReader(respBody, "project logo", 1)
	if err != nil {
		// Don't return on error but instead just continue and use the default image width and height
		slog.Warn("unable to parse svg image", "url", url, "error", err)
	}

	// Set defaults, then try to get the image's actual width and height
	imgWidth := defaultWidth
	imgHeight := defaultHeight
	if svgImg != nil {
		viewBox, err := svgImg.ViewBoxValues()
		if err != nil {
			imgWidth, _ = strconv.Atoi(svgImg.Width)
			imgHeight, _ = strconv.Atoi(svgImg.Height)
			slog.Warn("unable to get image dimensions from viewbox, so using height and width on the svg element", "url", url, "error", err)
		} else {
			imgWidth = int(viewBox[2])  // width
			imgHeight = int(viewBox[3]) // height
		}

		// Resort back to default dimensions if the width or height is 0, since in that case we wouldn't
		// know what the ratio of the width:height should be.
		if imgWidth == 0 || imgHeight == 0 {
			slog.Warn("one of the image dimensions is set to zero, so using default height and width", "url", url, "img_width", imgWidth, "img_height", imgHeight)
			imgWidth = defaultWidth
			imgHeight = defaultHeight
		}
	}

	// Write the body to file
	_, err = io.Copy(out, &buf)
	if err != nil {
		slog.Error("error copying response body into file", "url", url, "error", err)
		return nil, err
	}

	slog.Info("image download time",
		"url", url,
		"image_width", imgWidth,
		"image_height", imgHeight,
		"duration", time.Since(downloadImageTime).String())
	return &ImageDimensions{Width: imgWidth, Height: imgHeight}, nil
}

// convertFile converts an .svg file to .png format with a given width and height, from a specific input path to an output path
func convertFile(inputFilePath string, outputFilePath string, imageWidth int, imageHeight int) error {
	// Run command to export SVG as PNG with inkscape
	conversionTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		"inkscape",
		"--export-type=png",
		fmt.Sprintf("--export-width=%d", imageWidth),
		fmt.Sprintf("--export-height=%d", imageHeight),
		fmt.Sprintf("--export-filename=%s", outputFilePath),
		inputFilePath)
	slog.Debug("inkscape file conversion command", "cmd", cmd.String())

	if _, err := cmd.Output(); err != nil {
		slog.Error("inkscape: error running command", "error", err)
		return err
	}
	slog.Info("image conversion time",
		"input_file", inputFilePath,
		"output_file", outputFilePath,
		"duration", time.Since(conversionTime).String())

	return nil
}

// getFile tries to open a file and prints some file stats, used to check that a file exists
func getFile(filePath string) (*os.File, error) {
	file, err := os.Open(filePath)
	if err != nil {
		slog.Error("error opening file", "file", filePath, "error", err)
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		slog.Error("error getting file stats", "error", err)
		return nil, err
	}
	slog.Debug("converted file stats", "file", filePath, "size", stat.Size(), "mode", stat.Mode())

	return file, nil
}

// writeToS3Bucket writes a file to an s3 bucket on the account that the client is associated with.
// The bucket that it is written to is of a set name that is defined below.
func writeToS3Bucket(s3Client *s3.Client, file *os.File) error {
	bucketName := fmt.Sprintf("lfx-one-project-logos-png-%s", lfxEnvironment)
	keyName := file.Name()

	// Try to make the key name just the file name without the directories in the file path
	splitFileNamePath := strings.Split(file.Name(), "/")
	if len(splitFileNamePath) > 0 {
		keyName = splitFileNamePath[len(splitFileNamePath)-1]
	}

	slog.Debug("s3 upload details", "bucket", bucketName, "key", keyName)

	uploader := manager.NewUploader(s3Client)

	uploadImageTime := time.Now()
	output, err := uploader.Upload(context.Background(), &s3.PutObjectInput{
		Bucket: &bucketName,
		Key:    &keyName,
		Body:   file,
	})
	if err != nil || output == nil {
		slog.Error("error uploading file to s3 bucket", "file_name", file.Name(), "error", err)
		return err
	}
	slog.Info("image upload time", "location", output.Location, "duration", time.Since(uploadImageTime).String())

	return nil
}

func runSingleFile(s3Client *s3.Client, url string, imageWidth int, imageHeight int) error {
	// Create the local file
	out, err := createFile(fmt.Sprintf("./files/in-project-logo-%dx%d.svg", imageWidth, imageHeight))
	if err != nil {
		return fmt.Errorf("error creating local file: %s", url)
	}

	// Download the remote file into the local file
	origImgDimensions, err := downloadFile(url, out)
	if err != nil {
		return fmt.Errorf("error downloading remote file: %s", url)
	}
	out.Close()

	if imageWidth == 0 {
		// If there is no specified width for the new image file, use the proportions of the original image
		imgRatio := float64(origImgDimensions.Width) / float64(origImgDimensions.Height)
		imageWidth = int(float64(imageHeight) * imgRatio)
		slog.Debug("calculated adjusted png image width from original image height",
			"orig_image_height", origImgDimensions.Height,
			"orig_image_width", origImgDimensions.Width,
			"new_image_height", imageHeight,
			"new_image_width", imageWidth)
	}

	// Convert the svg file into a png file
	err = convertFile(
		fmt.Sprintf("./files/in-project-logo-%dx%d.svg", imageWidth, imageHeight),
		fmt.Sprintf("./files/out-project-logo-%dx%d.png", imageWidth, imageHeight),
		imageWidth,
		imageHeight)
	if err != nil {
		return fmt.Errorf("error converting svg file into png file: %s", url)
	}

	// Check that the png file now exists
	file, err := getFile(fmt.Sprintf("./files/out-project-logo-%dx%d.png", imageWidth, imageHeight))
	if err != nil {
		return fmt.Errorf("error confirming that new png file exists: %s", url)
	}

	if s3Client != nil {
		err = writeToS3Bucket(s3Client, file)
		if err != nil {
			return fmt.Errorf("error writing file to s3 bucket: %s", url)
		}
	}

	return nil
}

func runSelectProjectLogos(natsKV jetstream.KeyValue, s3Client *s3.Client, projectIds []string, imageWidth int, imageHeight int) error {
	if natsKV == nil {
		return errors.New("missing NATS KV store")
	}

	var (
		numProjectLogosConverted int
		numProjectsSkipped       int
	)

	// Fetch projects from NATS KV store
	projects, err := getAllProjects(natsKV)
	if err != nil {
		slog.Error("error fetching projects from NATS KV", "error", err)
		return err
	}

	paramImageWidth := imageWidth
	for _, project := range projects {
		// Skip unselected projects
		if !slices.Contains(projectIds, project.UID) {
			continue
		}

		// Skip projects that have no logo or that are not in svg format
		projectLogoUrl, err := url.Parse(project.LogoURL)
		if err != nil {
			slog.Error("error parsing project logo URL", "project_id", project.UID, "error", err)
			continue
		}
		slog.Debug("parsed project logo URL", "logo_url", project.LogoURL, "logo_url_path", projectLogoUrl.Path)
		if project.LogoURL == "" || !strings.HasSuffix(projectLogoUrl.Path, "svg") {
			numProjectsSkipped++
			slog.Debug("skipping project because there is either no logo or it is not in an svg file format", "project_id", project.UID, "project_logo", project.LogoURL)
			continue
		}

		// Create the local file
		out, err := createFile(fmt.Sprintf("./files/%s.svg", project.UID))
		if err != nil {
			slog.Error("error creating local file", "project_id", project.UID, "error", err)
			continue
		}

		// Download the remote file into the local file
		origImgDimensions, err := downloadFile(project.LogoURL, out)
		if err != nil {
			slog.Error("error downloading remote file", "project_id", project.UID, "error", err)
			continue
		}
		out.Close()

		if paramImageWidth == 0 {
			// If there is no specified width for the new image file, use the proportions of the original image
			imgRatio := float64(origImgDimensions.Width) / float64(origImgDimensions.Height)
			imageWidth = int(float64(imageHeight) * imgRatio)
			slog.Debug("calculated adjusted png image width from original image height",
				"orig_image_height", origImgDimensions.Height,
				"orig_image_width", origImgDimensions.Width,
				"new_image_height", imageHeight,
				"new_image_width", imageWidth)
		}

		// Convert the svg file into a png file
		err = convertFile(
			fmt.Sprintf("./files/%s.svg", project.UID),
			fmt.Sprintf("./files/%s.png", project.UID),
			imageWidth,
			imageHeight)
		if err != nil {
			slog.Error("error converting svg file into png file", "project_id", project.UID, "error", err)
			continue
		}

		// Check that the png file now exists
		file, err := getFile(fmt.Sprintf("./files/%s.png", project.UID))
		if err != nil {
			slog.Error("error confirming that new png file exists", "project_id", project.UID, "error", err)
			continue
		}

		if s3Client != nil {
			err = writeToS3Bucket(s3Client, file)
			if err != nil {
				slog.Error("error writing file to s3 bucket", "project_id", project.UID, "error", err)
				continue
			}
		}

		numProjectLogosConverted++
	}

	slog.Info("statistics about converted project logos",
		"num_project_logos_converted", numProjectLogosConverted,
		"num_projects_skipped", numProjectsSkipped)

	return nil
}

func runAllProjectLogos(natsKV jetstream.KeyValue, s3Client *s3.Client, imageWidth int, imageHeight int) error {
	if natsKV == nil {
		return errors.New("missing NATS KV store")
	}

	var (
		numProjectLogosConverted int
		numProjectsSkipped       int
	)

	// Fetch all projects from NATS KV store
	projects, err := getAllProjects(natsKV)
	if err != nil {
		slog.Error("error fetching projects from NATS KV")
		return err
	}

	paramImageWidth := imageWidth
	for _, project := range projects {
		// Skip projects that have no logo or that are not in svg format
		if project.LogoURL == "" || !strings.HasSuffix(project.LogoURL, "svg") {
			numProjectsSkipped++
			slog.Debug("skipping project because there is either no logo or it is not in an svg file format", "project_id", project.UID, "project_logo", project.LogoURL)
			continue
		}

		// Create the local file
		out, err := createFile(fmt.Sprintf("./files/%s.svg", project.UID))
		if err != nil {
			slog.Error("error creating local file", "project_id", project.UID, "error", err)
			continue
		}

		// Download the remote file into the local file
		origImgDimensions, err := downloadFile(project.LogoURL, out)
		if err != nil {
			slog.Error("error downloading remote file", "project_id", project.UID, "error", err)
			continue
		}
		out.Close()

		if paramImageWidth == 0 {
			// If there is no specified width for the new image file, use the proportions of the original image
			imgRatio := float64(origImgDimensions.Width) / float64(origImgDimensions.Height)
			imageWidth = int(float64(imageHeight) * imgRatio)
			slog.Debug("calculated adjusted png image width from original image height",
				"orig_image_height", origImgDimensions.Height,
				"orig_image_width", origImgDimensions.Width,
				"new_image_height", imageHeight,
				"new_image_width", imageWidth)
		}

		// Convert the svg file into a png file
		err = convertFile(
			fmt.Sprintf("./files/%s.svg", project.UID),
			fmt.Sprintf("./files/%s.png", project.UID),
			imageWidth,
			imageHeight)
		if err != nil {
			slog.Error("error converting svg file into png file", "project_id", project.UID, "error", err)
			continue
		}

		// Check that the png file now exists
		file, err := getFile(fmt.Sprintf("./files/%s.png", project.UID))
		if err != nil {
			slog.Error("error confirming that new png file exists", "project_id", project.UID, "error", err)
			continue
		}

		if s3Client != nil {
			err = writeToS3Bucket(s3Client, file)
			if err != nil {
				slog.Error("error writing file to s3 bucket", "project_id", project.UID, "error", err)
				continue
			}
		}

		numProjectLogosConverted++
	}

	slog.Info("statistics about converted project logos",
		"num_project_logos_converted", numProjectLogosConverted,
		"num_projects_skipped", numProjectsSkipped)

	return nil
}

func main() {
	optAllProjectLogos := flag.Bool("all-project-logos", false, "whether to fetch and convert all project logos to png file format")
	optSelectProjectLogos := flag.String("select-project-logos", "", "selection of project IDs of projects whose logos should be converted to png file format (e.g. a09P000000DsCBuISe,a09P000000DsCBuITr,a09P000000DsCBuILv)")
	optUrl := flag.String("url", "", "url of svg file to be converted to png file format")
	optWriteS3 := flag.Bool("write-s3", false, "whether to write to s3 bucket")
	optKeepFiles := flag.Bool("keep-files", true, "whether to keep the converted files stored locally")
	optOutputWidth := flag.Int("width", 0, "image width for converted file")
	optOutputHeight := flag.Int("height", 800, "image height for converted file")
	optDebug := flag.Bool("d", false, "whether to log debug level")

	flag.Usage = func() {
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()

	if optDebug != nil && *optDebug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	slog.Debug("program command-line options",
		"all_project_logos", *optAllProjectLogos,
		"select_project_logos", *optSelectProjectLogos,
		"url", *optUrl,
		"write_s3", *optWriteS3,
		"width", *optOutputWidth,
		"height", *optOutputHeight,
		"debug", *optDebug)

	// Create directory for storing downloaded and converted image files locally
	err := os.Mkdir("files", 0755)
	if err != nil && !os.IsExist(err) {
		slog.Error("error creating files directory", "error", err)
		return
	}

	// Delete directory if cli option is set to not keep the files locally
	if optKeepFiles != nil && !*optKeepFiles {
		defer func() {
			err := os.RemoveAll("./files/")
			if err != nil {
				slog.Error("error deleting files directory", "error", err)
				return
			}
		}()
	}

	var s3Client *s3.Client
	if optWriteS3 != nil && *optWriteS3 {
		s3Client, err = getS3Client()
		if err != nil {
			slog.Error("error creating s3 client", "error", err)
			return
		}
	}

	if optUrl != nil && *optUrl != "" {
		// Only convert a single file with the specified url of the file to be converted
		err := runSingleFile(s3Client, *optUrl, *optOutputWidth, *optOutputHeight)
		if err != nil {
			slog.Error("error converting single file", "url", *optUrl, "error", err)
			return
		}

		slog.Info("successfully converted single file", "url", *optUrl)
		return
	}

	if optSelectProjectLogos != nil && *optSelectProjectLogos != "" {
		projectIds := strings.Split(*optSelectProjectLogos, ",")

		// Connect to NATS and get projects KV store
		natsKV, err := getNatsProjectsKV()
		if err != nil {
			slog.Error("error connecting to NATS projects KV", "error", err)
			return
		}

		err = runSelectProjectLogos(natsKV, s3Client, projectIds, *optOutputWidth, *optOutputHeight)
		if err != nil {
			slog.Error("error converting selected project logos", "error", err)
			return
		}

		slog.Info("successfully converted selected project logos", "project_ids", projectIds)
		return
	}

	if optAllProjectLogos != nil && *optAllProjectLogos {
		// Connect to NATS and get projects KV store
		natsKV, err := getNatsProjectsKV()
		if err != nil {
			slog.Error("error connecting to NATS projects KV", "error", err)
			return
		}

		err = runAllProjectLogos(natsKV, s3Client, *optOutputWidth, *optOutputHeight)
		if err != nil {
			slog.Error("error converting all project logos", "error", err)
			return
		}

		slog.Info("successfully converted all project logos")
		return
	}
}

func getS3Client() (*s3.Client, error) {
	lfxEnvironment = getRequiredEnvVar("LFX_ENVIRONMENT") // global

	awsConfig, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		slog.Error("could not load aws config", "error", err)
		return nil, err
	}

	s3Client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		o.Region = "us-west-2"
		o.UsePathStyle = true
	})

	return s3Client, nil
}

// getNatsProjectsKV connects to NATS and returns the projects KV store
func getNatsProjectsKV() (jetstream.KeyValue, error) {
	natsURL := getRequiredEnvVar("NATS_URL")

	// Connect to NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	// Get the projects KV store
	kv, err := js.KeyValue(context.Background(), "projects")
	if err != nil {
		return nil, fmt.Errorf("failed to get projects KV store: %w", err)
	}

	return kv, nil
}

// getAllProjects fetches all projects from NATS KV store
func getAllProjects(kv jetstream.KeyValue) ([]ProjectBase, error) {
	var projects []ProjectBase

	// List all keys in the projects bucket
	lister, err := kv.ListKeys(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to list project keys: %w", err)
	}

	for key := range lister.Keys() {
		// Get the project data
		entry, err := kv.Get(context.Background(), key)
		if err != nil {
			slog.Warn("failed to get project data, skipping", "key", key, "error", err)
			continue
		}

		// Unmarshal the project data
		var project ProjectBase
		if err := json.Unmarshal(entry.Value(), &project); err != nil {
			slog.Warn("failed to unmarshal project data, skipping", "key", key, "error", err)
			continue
		}

		projects = append(projects, project)
	}

	return projects, nil
}

func getRequiredEnvVar(varName string) string {
	envVar := os.Getenv(varName)
	if envVar == "" {
		slog.Error("missing environment variable", "var_name", varName)
		os.Exit(1)
	}

	return envVar
}
