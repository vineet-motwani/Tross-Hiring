package models

import (
	"time"
)

type ProfileRequest struct {
	URL                string `json:"url"`
	IncludeContactInfo bool   `json:"include_contact_info"`
}

type DateValue struct {
	Year  *int `json:"year,omitempty"`
	Month *int `json:"month,omitempty"`
	Day   *int `json:"day,omitempty"`
}

type DateRange struct {
	Start   *DateValue `json:"start,omitempty"`
	End     *DateValue `json:"end,omitempty"`
	Present bool       `json:"present"`
}

type ImageAsset struct {
	URL    string `json:"url"`
	Width  *int   `json:"width,omitempty"`
	Height *int   `json:"height,omitempty"`
}

type ProfileImages struct {
	Profile    *ImageAsset `json:"profile,omitempty"`
	Background *ImageAsset `json:"background,omitempty"`
}

type Location struct {
	DisplayName string `json:"display_name,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
}

type Experience struct {
	ID             *string     `json:"id,omitempty"`
	Title          *string     `json:"title,omitempty"`
	CompanyName    *string     `json:"company_name,omitempty"`
	CompanyURL     *string     `json:"company_url,omitempty"`
	CompanyLogo    *ImageAsset `json:"company_logo,omitempty"`
	EmploymentType *string     `json:"employment_type,omitempty"`
	Location       *string     `json:"location,omitempty"`
	DateRange      *DateRange  `json:"date_range,omitempty"`
	Description    *string     `json:"description,omitempty"`
}

type Education struct {
	ID           *string     `json:"id,omitempty"`
	SchoolName   *string     `json:"school_name,omitempty"`
	SchoolURL    *string     `json:"school_url,omitempty"`
	SchoolLogo   *ImageAsset `json:"school_logo,omitempty"`
	DegreeName   *string     `json:"degree_name,omitempty"`
	FieldOfStudy *string     `json:"field_of_study,omitempty"`
	Grade        *string     `json:"grade,omitempty"`
	Activities   *string     `json:"activities,omitempty"`
	DateRange    *DateRange  `json:"date_range,omitempty"`
	Description  *string     `json:"description,omitempty"`
}

type Skill struct {
	ID               *string `json:"id,omitempty"`
	Name             string  `json:"name"`
	EndorsementCount *int    `json:"endorsement_count,omitempty"`
}

type Certification struct {
	ID                  *string    `json:"id,omitempty"`
	Name                *string    `json:"name,omitempty"`
	IssuingOrganization *string    `json:"issuing_organization,omitempty"`
	IssueDate           *DateValue `json:"issue_date,omitempty"`
	ExpirationDate      *DateValue `json:"expiration_date,omitempty"`
	CredentialID        *string    `json:"credential_id,omitempty"`
	CredentialURL       *string    `json:"credential_url,omitempty"`
}

type Language struct {
	ID          *string `json:"id,omitempty"`
	Name        string  `json:"name"`
	Proficiency *string `json:"proficiency,omitempty"`
}

type Project struct {
	ID          *string    `json:"id,omitempty"`
	Name        *string    `json:"name,omitempty"`
	Description *string    `json:"description,omitempty"`
	DateRange   *DateRange `json:"date_range,omitempty"`
	URL         *string    `json:"url,omitempty"`
}

type Publication struct {
	ID          *string    `json:"id,omitempty"`
	Name        *string    `json:"name,omitempty"`
	Publisher   *string    `json:"publisher,omitempty"`
	Description *string    `json:"description,omitempty"`
	PublishedOn *DateValue `json:"published_on,omitempty"`
	URL         *string    `json:"url,omitempty"`
}

type Course struct {
	ID     *string `json:"id,omitempty"`
	Name   *string `json:"name,omitempty"`
	Number *string `json:"number,omitempty"`
}

type Honor struct {
	ID          *string    `json:"id,omitempty"`
	Title       *string    `json:"title,omitempty"`
	Issuer      *string    `json:"issuer,omitempty"`
	Description *string    `json:"description,omitempty"`
	IssuedOn    *DateValue `json:"issued_on,omitempty"`
}

type VolunteerExperience struct {
	ID           *string    `json:"id,omitempty"`
	Role         *string    `json:"role,omitempty"`
	Organization *string    `json:"organization,omitempty"`
	Cause        *string    `json:"cause,omitempty"`
	Description  *string    `json:"description,omitempty"`
	DateRange    *DateRange `json:"date_range,omitempty"`
}

type ContactInfo struct {
	Email          *string                  `json:"email,omitempty"`
	PhoneNumbers   []map[string]interface{} `json:"phone_numbers"`
	Websites       []map[string]interface{} `json:"websites"`
	TwitterHandles []map[string]interface{} `json:"twitter_handles"`
}

type Profile struct {
	LinkedInID          *string               `json:"linkedin_id,omitempty"`
	PublicIdentifier    string                `json:"public_identifier"`
	ProfileURL          string                `json:"profile_url"`
	FirstName           *string               `json:"first_name,omitempty"`
	LastName            *string               `json:"last_name,omitempty"`
	FullName            *string               `json:"full_name,omitempty"`
	Headline            *string               `json:"headline,omitempty"`
	About               *string               `json:"about,omitempty"`
	Location            Location              `json:"location"`
	Industry            *string               `json:"industry,omitempty"`
	ConnectionDegree    *int                  `json:"connection_degree,omitempty"`
	FollowerCount       *int                  `json:"follower_count,omitempty"`
	ConnectionCount     *int                  `json:"connection_count,omitempty"`
	Images              ProfileImages         `json:"images"`
	Experience          []Experience          `json:"experience"`
	Education           []Education           `json:"education"`
	Skills              []Skill               `json:"skills"`
	Certifications      []Certification       `json:"certifications"`
	Languages           []Language            `json:"languages"`
	Projects            []Project             `json:"projects"`
	Publications        []Publication         `json:"publications"`
	Courses             []Course              `json:"courses"`
	Honors              []Honor               `json:"honors"`
	VolunteerExperience []VolunteerExperience `json:"volunteer_experience"`
	ContactInfo         *ContactInfo          `json:"contact_info,omitempty"`
}

type ResponseMeta struct {
	SchemaVersion string    `json:"schema_version"`
	RetrievedAt   time.Time `json:"retrieved_at"`
	Source        string    `json:"source"`
	Cached        bool      `json:"cached"`
	Partial       bool      `json:"partial"`
	Warnings      []string  `json:"warnings"`
}

type ProfileResponse struct {
	Meta    ResponseMeta `json:"meta"`
	Profile Profile      `json:"profile"`
}

type ErrorDetail struct {
	Code      string  `json:"code"`
	Message   string  `json:"message"`
	RequestID *string `json:"request_id,omitempty"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}
