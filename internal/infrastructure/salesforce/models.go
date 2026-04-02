// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

// soqlAsset represents a Salesforce Asset record returned by a SOQL query.
// Relationship fields (Account, Product2, Project) are populated via SOQL
// relationship sub-selects. The Contact sub-select is intentionally omitted —
// the billing contact on an Asset is not a key contact; key contacts are fetched
// separately via Project_Role__c queries.
type soqlAsset struct {
	ID               string            `salesforce:"Id"`
	Name             string            `salesforce:"Name"`
	Status           *string           `salesforce:"Status"`
	AccountID        string            `salesforce:"AccountId"`
	Product2ID       string            `salesforce:"Product2Id"`
	IsDeleted        bool              `salesforce:"IsDeleted"`
	Year             *string           `salesforce:"Year__c"`
	Tier             *string           `salesforce:"Tier__c"`
	RecordTypeID     *string           `salesforce:"RecordTypeId"`
	AutoRenew        bool              `salesforce:"Auto_Renew__c"`
	RenewalType      *string           `salesforce:"Renewal_Type__c"`
	Price            float64           `salesforce:"Price"`
	AnnualFullPrice  float64           `salesforce:"Annual_Full_Price__c"`
	PaymentFrequency *string           `salesforce:"PaymentFrequency__c"`
	PaymentTerms     *string           `salesforce:"PaymentTerms__c"`
	AgreementDate    *string           `salesforce:"Agreement_Date__c"`
	PurchaseDate     *string           `salesforce:"PurchaseDate"`
	InstallDate      *string           `salesforce:"InstallDate"`
	UsageEndDate     *string           `salesforce:"UsageEndDate"`
	ProjectsID       *string           `salesforce:"Projects__c"`
	CreatedDate      string            `salesforce:"CreatedDate"`
	LastModifiedDate string            `salesforce:"LastModifiedDate"`
	Account          *soqlAssetAccount `salesforce:"Account"`
	Product2         *soqlAssetProduct `salesforce:"Product2"`
	Project          *soqlAssetProject `salesforce:"Projects__r"`
}

// soqlAssetAccount is the inline Account relationship on an Asset query.
type soqlAssetAccount struct {
	ID      string  `salesforce:"Id"`
	Name    string  `salesforce:"Name"`
	LogoURL *string `salesforce:"Logo_URL__c"`
	Website *string `salesforce:"Website"`
}

// soqlAssetProduct is the inline Product2 relationship on an Asset query.
type soqlAssetProduct struct {
	ID     string  `salesforce:"Id"`
	Name   string  `salesforce:"Name"`
	Family *string `salesforce:"Family"`
	Type   *string `salesforce:"Type__c"`
}

// soqlAssetProject is the inline Project__c relationship on an Asset query.
type soqlAssetProject struct {
	ID      string  `salesforce:"Id"`
	Name    string  `salesforce:"Name"`
	LogoURL *string `salesforce:"Project_Logo__c"`
	Slug    *string `salesforce:"Slug__c"`
	Status  *string `salesforce:"Status__c"`
}

// soqlProjectRole represents a Salesforce Project_Role__c record returned by a
// SOQL query. This is the key contact object linking a Contact, Account, Asset,
// and Project with a role designation.
type soqlProjectRole struct {
	ID             string                  `salesforce:"Id"`
	AssetID        string                  `salesforce:"Asset__c"`
	ContactID      *string                 `salesforce:"Contact__c"`
	Role           *string                 `salesforce:"Role__c"`
	Status         *string                 `salesforce:"Status__c"`
	BoardMember    bool                    `salesforce:"BoardMember__c"`
	PrimaryContact bool                    `salesforce:"PrimaryContact__c"`
	IsDeleted      bool                    `salesforce:"IsDeleted"`
	CreatedDate    string                  `salesforce:"CreatedDate"`
	SystemModstamp string                  `salesforce:"SystemModstamp"`
	Contact        *soqlProjectRoleContact `salesforce:"Contact__r"`
	Asset          *soqlProjectRoleAsset   `salesforce:"Asset__r"`
}

// soqlProjectRoleContact is the inline Contact relationship on a Project_Role__c
// query. Only identity and personal attributes are fetched here; company
// (Account) data is sourced from the Asset relationship instead, so it is
// consistent with the associated ProjectMembership record.
type soqlProjectRoleContact struct {
	ID        string  `salesforce:"Id"`
	FirstName *string `salesforce:"FirstName"`
	LastName  *string `salesforce:"LastName"`
	Title     *string `salesforce:"Title"`
	Email     *string `salesforce:"Email"`
}

// soqlProjectRoleAssetAccount is the inline Account on the Asset relationship in
// a Project_Role__c query (Asset__r.Account). Company data for key contacts is
// sourced here rather than from Contact__r.Account so it matches the membership.
type soqlProjectRoleAssetAccount struct {
	ID      string  `salesforce:"Id"`
	Name    string  `salesforce:"Name"`
	LogoURL *string `salesforce:"Logo_URL__c"`
	Website *string `salesforce:"Website"`
}

// soqlProjectRoleAsset is the inline Asset relationship on a Project_Role__c
// query. Includes the Asset's Account and Project relationships so that company
// and project data can be denormalized onto the key contact record.
type soqlProjectRoleAsset struct {
	ID         string                       `salesforce:"Id"`
	AccountID  string                       `salesforce:"AccountId"`
	Product2ID string                       `salesforce:"Product2Id"`
	ProjectsID *string                      `salesforce:"Projects__c"`
	Account    *soqlProjectRoleAssetAccount `salesforce:"Account"`
	Project    *soqlAssetProject            `salesforce:"Projects__r"`
}

// soqlAlternateEmail represents a Salesforce Alternate_Email__c record.
// ContactID is sourced from Contact_Name__c, which is the lookup relationship
// field holding the full 18-char Contact SFID (not a human-readable name).
type soqlAlternateEmail struct {
	ID        string `salesforce:"Id"`
	ContactID string `salesforce:"Contact_Name__c"`
	Email     string `salesforce:"Alternate_Email_Address__c"`
	Primary   bool   `salesforce:"Primary_Email__c"`
}

// soqlAlternateEmailContact is the result of a contact lookup via
// Alternate_Email__c. Contact_Name__c is a lookup field holding the B2B
// Contact SFID.
type soqlAlternateEmailContact struct {
	ContactNameID string `salesforce:"Contact_Name__c"`
}

// soqlContactByEmail is the result of a contact lookup via Contact.Email.
type soqlContactByEmail struct {
	ID string `salesforce:"Id"`
}

// soqlProject represents a Salesforce Project__c record used for slug-based
// lookups against the Project__c object.
type soqlProject struct {
	ID   string  `salesforce:"Id"`
	Name string  `salesforce:"Name"`
	Slug *string `salesforce:"Slug__c"`
}

// soqlAccount represents a Salesforce Account record returned by a SOQL query.
// Used for B2BOrg search and list operations. The IsDeleted field is always
// false in results (filtered in the WHERE clause) and is included only to
// satisfy the go-salesforce library's struct requirements.
type soqlAccount struct {
	ID               string  `salesforce:"Id"`
	Name             string  `salesforce:"Name"`
	LogoURL          *string `salesforce:"Logo_URL__c"`
	Website          *string `salesforce:"Website"`
	CreatedDate      string  `salesforce:"CreatedDate"`
	LastModifiedDate string  `salesforce:"LastModifiedDate"`
}

// derefString returns the string value of a *string pointer, or an empty string
// if the pointer is nil.
func derefString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}
