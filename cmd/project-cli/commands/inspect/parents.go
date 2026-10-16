// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

// Finding kinds reported by inspect parents.
const (
	kindMissingParent = "missing_parent"
	kindSelfParent    = "self_parent"
	kindParentCycle   = "parent_cycle"
)

// projectLister is the slice of the repository the parent check reads.
type projectLister interface {
	ListAllProjectsBase(ctx context.Context) ([]*models.ProjectBase, error)
}

type parentsSubcommand struct{}

func (s *parentsSubcommand) Name() string { return "parents" }

func (s *parentsSubcommand) Help() string {
	return "find projects whose parent does not exist, is the project itself, or forms a cycle"
}

func (s *parentsSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := commands.NewFlagSet(s.Name(), "inspect parents [flags]")
	if help, err := commands.ParseFlags(fs, rc.Args); help || err != nil {
		return err
	}
	if err := commands.RejectExtraArgs(fs); err != nil {
		return err
	}

	natsConn, repo, err := commands.ConnectRepository(ctx, rc.NATSConfig)
	if err != nil {
		return err
	}
	defer natsConn.Close()

	return runParents(ctx, repo, os.Stdout)
}

type parentsReport struct {
	reportStatus
	Projects int            `json:"projects"`
	Counts   map[string]int `json:"counts"`
	Findings []finding      `json:"findings"`
}

// runParents checks the parent_uid chain of every project. A bad parent breaks
// the access a project inherits from its ancestors.
func runParents(ctx context.Context, repo projectLister, out io.Writer) error {
	bases, err := repo.ListAllProjectsBase(ctx)
	if err != nil {
		return fmt.Errorf("list project bases: %w", err)
	}

	parentOf := make(map[string]string, len(bases))
	slugOf := make(map[string]string, len(bases))
	for _, base := range bases {
		parentOf[base.UID] = base.ParentUID
		slugOf[base.UID] = base.Slug
	}

	report := &parentsReport{Projects: len(bases), Counts: map[string]int{}, Findings: []finding{}}
	add := func(f finding) {
		report.Findings = append(report.Findings, f)
		report.Counts[f.Kind]++
	}

	// A parent chain that is not broken ends at a project with no parent. Walk each
	// chain once; a chain that returns to a project already on the current walk is
	// a cycle, reported once, at its lowest UID.
	const (
		onWalk = 1
		done   = 2
	)
	state := make(map[string]int, len(bases))
	uids := make([]string, 0, len(bases))
	for uid := range parentOf {
		uids = append(uids, uid)
	}
	sort.Strings(uids)

	for _, start := range uids {
		var walk []string
		for uid := start; uid != ""; {
			if state[uid] == done {
				break
			}
			if state[uid] == onWalk {
				cycle := walk[indexOf(walk, uid):]
				if len(cycle) > 1 {
					lowest := cycle[0]
					for _, member := range cycle {
						if member < lowest {
							lowest = member
						}
					}
					add(finding{Kind: kindParentCycle, Slug: slugOf[lowest], UID: lowest, Detail: fmt.Sprintf("parent chain loops through %d projects", len(cycle))})
				}
				break
			}
			state[uid] = onWalk
			walk = append(walk, uid)

			parent := parentOf[uid]
			if parent == uid {
				add(finding{Kind: kindSelfParent, Slug: slugOf[uid], UID: uid, Detail: "parent_uid is the project itself"})
				parent = ""
			} else if _, ok := parentOf[parent]; parent != "" && !ok {
				add(finding{Kind: kindMissingParent, Slug: slugOf[uid], UID: uid, Detail: fmt.Sprintf("parent %q does not exist", parent)})
				parent = ""
			}
			uid = parent
		}
		for _, uid := range walk {
			state[uid] = done
		}
	}
	sortFindings(report.Findings)

	report.reportStatus = statusFor(len(report.Findings))
	return finishReport(ctx, "inspect parents", out, report, len(report.Findings), fmt.Sprintf("%d projects", report.Projects))
}

func indexOf(items []string, target string) int {
	for i, item := range items {
		if item == target {
			return i
		}
	}
	return -1
}
