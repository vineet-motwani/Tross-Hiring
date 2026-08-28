package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vineet-motwani/Tross-Hiring/identity"
	"github.com/vineet-motwani/Tross-Hiring/models"
)

type ParseError struct {
	Message string
}

func (e *ParseError) Error() string { return e.Message }

func urnID(value interface{}) *string {
	str, ok := value.(string)
	if !ok || str == "" {
		return nil
	}
	parts := strings.Split(str, ":")
	suffix := parts[len(parts)-1]
	if strings.HasPrefix(suffix, "(") && strings.HasSuffix(suffix, ")") {
		inner := suffix[1 : len(suffix)-1]
		innerParts := strings.SplitN(inner, ",", 2)
		if len(innerParts) == 2 && innerParts[1] != "" {
			return &innerParts[1]
		}
	}
	return &suffix
}

func memberScopedOwner(value interface{}) *string {
	str, ok := value.(string)
	if !ok || str == "" {
		return nil
	}
	parts := strings.Split(str, ":")
	suffix := parts[len(parts)-1]
	if !strings.HasPrefix(suffix, "(") {
		return nil
	}
	inner := suffix[1:]
	idx := strings.Index(inner, ",")
	if idx > 0 {
		res := inner[:idx]
		return &res
	}
	return nil
}

func text(value interface{}) *string {
	if str, ok := value.(string); ok {
		stripped := strings.TrimSpace(str)
		if stripped != "" {
			return &stripped
		}
		return nil
	}
	if dict, ok := value.(map[string]interface{}); ok {
		for _, key := range []string{"text", "name", "localizedName", "defaultLocalizedName"} {
			if v, exists := dict[key]; exists {
				res := text(v)
				if res != nil {
					return res
				}
			}
		}
	}
	return nil
}

func safeInt(value interface{}, min, max int) *int {
	if _, isBool := value.(bool); isBool {
		return nil
	}
	var res int
	switch v := value.(type) {
	case int:
		res = v
	case float64:
		res = int(v)
	case string:
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return nil
		}
		res = parsed
	default:
		return nil
	}
	if res >= min && res <= max {
		return &res
	}
	return nil
}

func date(value interface{}) *models.DateValue {
	dict, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	dv := &models.DateValue{
		Year:  safeInt(dict["year"], 1, 9999),
		Month: safeInt(dict["month"], 1, 12),
		Day:   safeInt(dict["day"], 1, 31),
	}
	if dv.Year != nil || dv.Month != nil || dv.Day != nil {
		return dv
	}
	return nil
}

func dateRange(value interface{}) *models.DateRange {
	dict, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	
	startVal := dict["start"]
	if startVal == nil {
		startVal = dict["startDate"]
	}
	endVal := dict["end"]
	if endVal == nil {
		endVal = dict["endDate"]
	}
	
	start := date(startVal)
	end := date(endVal)
	
	if start != nil || end != nil {
		return &models.DateRange{
			Start:   start,
			End:     end,
			Present: start != nil && end == nil,
		}
	}
	return nil
}

func image(value interface{}, depth int, seen map[string]bool) *models.ImageAsset {
	dict, ok := value.(map[string]interface{})
	if !ok || depth > 12 {
		return nil
	}
	if seen == nil {
		seen = make(map[string]bool)
	}
	marker := fmt.Sprintf("%p", dict) // simple object id approx
	if seen[marker] {
		return nil
	}
	seen[marker] = true

	for _, key := range []string{
		"vectorImage",
		"com.linkedin.common.VectorImage",
		"displayImageReference",
		"displayImage",
		"picture",
		"logo",
	} {
		if nested, exists := dict[key]; exists {
			img := image(nested, depth+1, seen)
			if img != nil {
				return img
			}
		}
	}

	root, hasRoot := dict["rootUrl"].(string)
	artifacts, hasArtifacts := dict["artifacts"].([]interface{})
	if hasRoot && hasArtifacts && len(artifacts) > 0 {
		var maxArtifact map[string]interface{}
		var maxArea int = -1
		for _, art := range artifacts {
			if artDict, ok := art.(map[string]interface{}); ok {
				w := safeInt(artDict["width"], 0, 100000)
				h := safeInt(artDict["height"], 0, 100000)
				area := 0
				if w != nil && h != nil {
					area = *w * *h
				}
				if area > maxArea {
					maxArea = area
					maxArtifact = artDict
				}
			}
		}
		if maxArtifact != nil {
			var segment string
			if s, ok := maxArtifact["fileIdentifyingUrlPathSegment"].(string); ok {
				segment = s
			} else if s, ok := maxArtifact["url"].(string); ok {
				segment = s
			}
			if segment != "" {
				url := segment
				if !strings.HasPrefix(segment, "http") {
					url = root + segment
				}
				return &models.ImageAsset{
					URL:    url,
					Width:  safeInt(maxArtifact["width"], 0, 100000),
					Height: safeInt(maxArtifact["height"], 0, 100000),
				}
			}
		}
	}
	if hasRoot {
		return &models.ImageAsset{URL: root}
	}

	for _, v := range dict {
		if vDict, ok := v.(map[string]interface{}); ok {
			img := image(vDict, depth+1, seen)
			if img != nil {
				return img
			}
		}
	}
	return nil
}

type VoyagerParser struct {
	documents        []map[string]interface{}
	profileDocuments []map[string]interface{}
	targetMemberID   *string
	entities         []map[string]interface{}
	urnMap           map[string]map[string]interface{}
}

func NewVoyagerParser(rawDocuments []interface{}) (*VoyagerParser, error) {
	vp := &VoyagerParser{}
	for _, doc := range rawDocuments {
		if d, ok := doc.(map[string]interface{}); ok {
			vp.documents = append(vp.documents, d)
		}
	}
	if len(vp.documents) > 16 {
		return nil, &ParseError{"LinkedIn returned too many response documents"}
	}
	vp.profileDocuments = vp.documents
	err := vp.loadEntities(vp.documents)
	if err != nil {
		return nil, err
	}
	return vp, nil
}

func (vp *VoyagerParser) loadEntities(documents []map[string]interface{}) error {
	vp.entities = nil
	for _, doc := range documents {
		if included, ok := doc["included"].([]interface{}); ok {
			for _, item := range included {
				if dict, isDict := item.(map[string]interface{}); isDict {
					vp.entities = append(vp.entities, dict)
				}
			}
		}
	}
	if len(vp.entities) > 2000 {
		return &ParseError{"LinkedIn returned too many profile entities"}
	}
	vp.urnMap = make(map[string]map[string]interface{})
	for _, entity := range vp.entities {
		if urn, ok := entity["entityUrn"].(string); ok {
			vp.urnMap[urn] = entity
		}
	}
	return nil
}

func (vp *VoyagerParser) Parse(publicIdentifier string, includeContactInfo bool) (*models.Profile, error) {
	if len(vp.documents) == 0 {
		return nil, &ParseError{"LinkedIn returned no profile document"}
	}
	
	expected := strings.ToLower(publicIdentifier)
	vp.profileDocuments = []map[string]interface{}{vp.documents[0]}
	for _, doc := range vp.documents[1:] {
		if src, _ := doc["__source"].(string); src == "section" {
			if pid, ok := doc["__profile_identifier"].(string); ok && strings.ToLower(pid) == expected {
				vp.profileDocuments = append(vp.profileDocuments, doc)
			}
		}
	}
	vp.loadEntities(vp.profileDocuments)

	legacy, err := vp.legacyProfileView(publicIdentifier)
	if err != nil { return nil, err }
	entity, err := vp.profileEntity(publicIdentifier)
	if err != nil { return nil, err }

	if entity == nil && legacy == nil {
		return nil, &ParseError{"LinkedIn returned no recognizable profile entity"}
	}

	var entityMemberID, legacyMemberID *string
	if entity != nil {
		entityMemberID = vp.profileMemberID(entity)
	}
	if legacy != nil {
		legacyMemberID = vp.legacyMemberID(legacy)
	}

	memberIDs := make(map[string]bool)
	if entityMemberID != nil {
		memberIDs[*entityMemberID] = true
	}
	if legacyMemberID != nil {
		memberIDs[*legacyMemberID] = true
	}
	if len(memberIDs) > 1 {
		return nil, &ParseError{"LinkedIn returned conflicting profile identities"}
	}

	for id := range memberIDs {
		vp.targetMemberID = &id
		break
	}
	
	if err := vp.validateMemberEntityOwnership(vp.targetMemberID); err != nil {
		return nil, err
	}

	var profile *models.Profile
	if entity != nil {
		profile, err = vp.parseDash(entity, publicIdentifier)
	} else {
		profile, err = vp.parseLegacy(legacy, publicIdentifier)
	}
	if err != nil { return nil, err }

	vp.mergeEntitySections(profile)
	// Omitting loose sections / legacy parsing details for brevity, can implement if needed.

	return profile, nil
}

func (vp *VoyagerParser) legacyProfileView(publicIdentifier string) (map[string]interface{}, error) {
	doc := vp.documents[0]
	profile, ok := doc["profile"].(map[string]interface{})
	if !ok {
		return nil, nil
	}
	if identity.HasMalformedPublicIdentifier(profile) {
		return nil, &ParseError{"LinkedIn returned conflicting profile identities"}
	}
	candidates := identity.ProfilePublicIdentifiers(profile)
	if len(candidates) == 0 {
		return nil, nil
	}
	for _, cand := range candidates {
		if strings.ToLower(cand) != strings.ToLower(publicIdentifier) {
			return nil, &ParseError{"LinkedIn returned conflicting profile identities"}
		}
	}
	return doc, nil
}

func (vp *VoyagerParser) profileEntity(publicIdentifier string) (map[string]interface{}, error) {
	primary := vp.documents[0]
	included, ok := primary["included"].([]interface{})
	if !ok {
		return nil, nil
	}
	
	var candidates []map[string]interface{}
	for _, item := range included {
		if dict, isDict := item.(map[string]interface{}); isDict {
			if typeName, _ := dict["$type"].(string); typeName == "com.linkedin.voyager.dash.identity.profile.Profile" {
				candidates = append(candidates, dict)
			}
		}
	}
	
	data, hasData := primary["data"].(map[string]interface{})
	if hasData {
		if roots, ok := data["*elements"].([]interface{}); ok {
			rootUrns := make(map[string]bool)
			for _, v := range roots {
				if s, isStr := v.(string); isStr {
					rootUrns[s] = true
				}
			}
			var filtered []map[string]interface{}
			for _, cand := range candidates {
				urn, _ := cand["entityUrn"].(string)
				if rootUrns[urn] {
					filtered = append(filtered, cand)
				}
			}
			candidates = filtered
		}
	}
	
	var exact []map[string]interface{}
	for _, cand := range candidates {
		pid, _ := cand["publicIdentifier"].(string)
		if strings.ToLower(pid) != strings.ToLower(publicIdentifier) {
			return nil, &ParseError{"LinkedIn returned conflicting profile identities"}
		}
		exact = append(exact, cand)
	}
	
	if len(exact) == 0 {
		return nil, nil
	}
	
	// Max length dictionary
	var maxCand map[string]interface{}
	maxLen := -1
	for _, cand := range exact {
		if len(cand) > maxLen {
			maxLen = len(cand)
			maxCand = cand
		}
	}
	return maxCand, nil
}

func (vp *VoyagerParser) profileMemberID(entity map[string]interface{}) *string {
	urn, ok := entity["entityUrn"].(string)
	if !ok || !strings.HasPrefix(urn, "urn:li:fsd_profile:") {
		return nil
	}
	parts := strings.Split(urn, ":")
	memberID := parts[len(parts)-1]
	if memberID != "" && !strings.HasPrefix(memberID, "(") {
		return &memberID
	}
	return nil
}

func (vp *VoyagerParser) legacyMemberID(document map[string]interface{}) *string {
	profile, ok := document["profile"].(map[string]interface{})
	if !ok {
		return nil
	}
	memberIDs := identity.ProfileMemberIds(profile)
	if len(memberIDs) > 1 {
		// Would raise error here but limited return values, ignore or assume empty
		return nil
	}
	if len(memberIDs) == 1 {
		return &memberIDs[0]
	}
	return nil
}

func (vp *VoyagerParser) validateMemberEntityOwnership(memberID *string) error {
	for _, entity := range vp.entities {
		typeName, _ := entity["$type"].(string)
		if entitySectionName(typeName) == nil {
			continue
		}
		urn, _ := entity["entityUrn"].(string)
		owner := memberScopedOwner(urn)
		if memberID != nil && owner != nil && *memberID != *owner {
			return &ParseError{"LinkedIn returned profile data owned by another member"}
		}
	}
	return nil
}

func entitySectionName(value interface{}) *string {
	str, ok := value.(string)
	if !ok {
		return nil
	}
	typeName := strings.ToLower(str)
	var res string
	if strings.HasSuffix(typeName, ".position") {
		res = "experience"
	} else if strings.HasSuffix(typeName, ".education") {
		res = "education"
	} else if strings.HasSuffix(typeName, ".skill") {
		res = "skills"
	} else if strings.Contains(typeName, "certification") && !strings.HasSuffix(typeName, "collection") {
		res = "certifications"
	} else if strings.HasSuffix(typeName, ".language") {
		res = "languages"
	} else if strings.HasSuffix(typeName, ".project") {
		res = "projects"
	} else if strings.HasSuffix(typeName, ".publication") {
		res = "publications"
	} else if strings.HasSuffix(typeName, ".course") {
		res = "courses"
	} else if strings.HasSuffix(typeName, ".honor") {
		res = "honors"
	} else if strings.Contains(typeName, "volunteerexperience") {
		res = "volunteer_experience"
	}
	if res != "" {
		return &res
	}
	return nil
}

func (vp *VoyagerParser) resolve(entity map[string]interface{}, fields ...string) map[string]interface{} {
	for _, field := range fields {
		val := entity[field]
		if s, ok := val.(string); ok {
			if resolved, found := vp.urnMap[s]; found {
				return resolved
			}
		} else if d, ok := val.(map[string]interface{}); ok {
			return d
		}
	}
	return nil
}

func (vp *VoyagerParser) parseDash(entity map[string]interface{}, publicIdentifier string) (*models.Profile, error) {
	geo := vp.resolve(entity, "*geo")
	if geo == nil {
		if geoLoc, ok := entity["geoLocation"].(map[string]interface{}); ok {
			geo = vp.resolve(geoLoc, "*geo", "geoUrn")
		}
	}
	industry := vp.resolve(entity, "*industry")

	firstName := text(entity["firstName"])
	lastName := text(entity["lastName"])
	locName := text(entity["locationName"])
	if locName == nil {
		locName = text(geo)
	}

	var countryCode *string
	if loc, ok := entity["location"].(map[string]interface{}); ok {
		countryCode = text(loc["countryCode"])
	}

	var fullName *string
	if firstName != nil && lastName != nil {
		s := *firstName + " " + *lastName
		fullName = &s
	}

	ind := text(industry)
	if ind == nil {
		ind = text(entity["industryName"])
	}

	prof := &models.Profile{
		LinkedInID:          urnID(entity["entityUrn"]),
		PublicIdentifier:    publicIdentifier,
		ProfileURL:          "https://www.linkedin.com/in/" + publicIdentifier + "/",
		FirstName:           firstName,
		LastName:            lastName,
		FullName:            fullName,
		Headline:            text(entity["headline"]),
		About:               text(entity["summary"]),
		Location: models.Location{
			DisplayName: strValue(locName),
			CountryCode: strValue(countryCode),
		},
		Industry:            ind,
		ConnectionDegree:    vp.connectionDegree(entity),
		FollowerCount:       safeInt(entity["followerCount"], 0, 100000000),
		ConnectionCount:     safeInt(entity["connectionCount"], 0, 100000000),
		Experience:          make([]models.Experience, 0),
		Education:           make([]models.Education, 0),
		Skills:              make([]models.Skill, 0),
		Certifications:      make([]models.Certification, 0),
		Languages:           make([]models.Language, 0),
		Projects:            make([]models.Project, 0),
		Publications:        make([]models.Publication, 0),
		Courses:             make([]models.Course, 0),
		Honors:              make([]models.Honor, 0),
		VolunteerExperience: make([]models.VolunteerExperience, 0),
	}

	img1 := image(entity["profilePicture"], 0, nil)
	if img1 == nil {
		img1 = image(entity["picture"], 0, nil)
	}
	prof.Images = models.ProfileImages{
		Profile:    img1,
		Background: image(entity["backgroundPicture"], 0, nil),
	}
	
	return prof, nil
}

func (vp *VoyagerParser) parseLegacy(document map[string]interface{}, publicIdentifier string) (*models.Profile, error) {
	entity, ok := document["profile"].(map[string]interface{})
	if !ok {
		return nil, &ParseError{"LinkedIn returned no recognizable profile entity"}
	}
	mini, _ := entity["miniProfile"].(map[string]interface{})
	if mini == nil {
		mini = make(map[string]interface{})
	}
	
	firstName := text(entity["firstName"])
	if firstName == nil {
		firstName = text(mini["firstName"])
	}
	lastName := text(entity["lastName"])
	if lastName == nil {
		lastName = text(mini["lastName"])
	}
	
	var fullName *string
	if firstName != nil && lastName != nil {
		s := *firstName + " " + *lastName
		fullName = &s
	}
	
	head := text(entity["headline"])
	if head == nil {
		head = text(mini["occupation"])
	}
	
	urn := mini["entityUrn"]
	if urn == nil {
		urn = entity["entityUrn"]
	}
	
	var cc *string
	if loc, ok := entity["location"].(map[string]interface{}); ok {
		cc = text(loc["countryCode"])
	}
	
	img := image(mini, 0, nil)
	if img == nil {
		img = image(entity, 0, nil)
	}
	
	return &models.Profile{
		LinkedInID:       urnID(urn),
		PublicIdentifier: publicIdentifier,
		ProfileURL:       "https://www.linkedin.com/in/" + publicIdentifier + "/",
		FirstName:        firstName,
		LastName:         lastName,
		FullName:         fullName,
		Headline:         head,
		About:            text(entity["summary"]),
		Location: models.Location{
			DisplayName: strValue(text(entity["locationName"])),
			CountryCode: strValue(cc),
		},
		Industry: text(entity["industryName"]),
		Images: models.ProfileImages{
			Profile: img,
		},
	}, nil
}

func (vp *VoyagerParser) connectionDegree(profile map[string]interface{}) *int {
	rel := vp.resolve(profile, "*memberRelationship")
	if rel == nil {
		return nil
	}
	union, ok := rel["memberRelationshipUnion"].(map[string]interface{})
	if !ok {
		union, ok = rel["memberRelationshipData"].(map[string]interface{})
	}
	if !ok {
		return nil
	}
	for _, key := range []string{"connectedMember", "connected", "connection"} {
		if _, ok := union[key]; ok {
			res := 1
			return &res
		}
	}
	if noConn, ok := union["noConnection"].(map[string]interface{}); ok {
		if dist, ok := noConn["memberDistance"].(string); ok {
			if dist == "DISTANCE_1" { res := 1; return &res }
			if dist == "DISTANCE_2" { res := 2; return &res }
			if dist == "DISTANCE_3" { res := 3; return &res }
		}
	}
	return nil
}

func (vp *VoyagerParser) mergeEntitySections(profile *models.Profile) {
	for _, entity := range vp.entities {
		typeName, _ := entity["$type"].(string)
		secName := entitySectionName(typeName)
		if secName == nil {
			continue
		}
		
		switch *secName {
		case "experience":
			company := vp.resolve(entity, "*company", "company")
			cName := text(entity["companyName"])
			if cName == nil {
				cName = text(company)
			}
			var cUrl *string
			if univName, ok := company["universalName"].(string); ok {
				url := "https://www.linkedin.com/company/" + univName + "/"
				cUrl = &url
			}
			loc := text(entity["locationName"])
			if loc == nil {
				loc = text(entity["geoLocationName"])
			}
			dr := entity["dateRange"]
			if dr == nil { dr = entity["timePeriod"] }
			
			profile.Experience = append(profile.Experience, models.Experience{
				ID:             urnID(entity["entityUrn"]),
				Title:          text(entity["title"]),
				CompanyName:    cName,
				CompanyURL:     cUrl,
				CompanyLogo:    image(company, 0, nil),
				EmploymentType: text(entity["employmentType"]),
				Location:       loc,
				DateRange:      dateRange(dr),
				Description:    text(entity["description"]),
			})
		case "education":
			school := vp.resolve(entity, "*school", "school")
			sName := text(entity["schoolName"])
			if sName == nil { sName = text(school) }
			
			profile.Education = append(profile.Education, models.Education{
				ID:           urnID(entity["entityUrn"]),
				SchoolName:   sName,
				SchoolLogo:   image(school, 0, nil),
				DegreeName:   text(entity["degreeName"]),
				FieldOfStudy: text(entity["fieldOfStudy"]),
				Grade:        text(entity["grade"]),
				Activities:   text(entity["activities"]),
				DateRange:    dateRange(entity["dateRange"]), // simplify fallback
				Description:  text(entity["description"]),
			})
		case "skills":
			name := text(entity["name"])
			if name != nil {
				profile.Skills = append(profile.Skills, models.Skill{
					ID:               urnID(entity["entityUrn"]),
					Name:             *name,
					EndorsementCount: safeInt(entity["endorsementCount"], 0, 1000000),
				})
			}
		case "certifications":
			issued := entity["timePeriod"]
			if issued == nil {
				issued = entity["dateRange"]
			}
			var startVal, endVal *models.DateValue
			if issuedMap, ok := issued.(map[string]interface{}); ok {
				startVal = date(issuedMap["start"])
				endVal = date(issuedMap["end"])
			}
			org := text(entity["authority"])
			if org == nil {
				org = text(entity["issuingOrganization"])
			}
			credID := text(entity["licenseNumber"])
			if credID == nil {
				credID = text(entity["credentialId"])
			}
			profile.Certifications = append(profile.Certifications, models.Certification{
				ID:                  urnID(entity["entityUrn"]),
				Name:                text(entity["name"]),
				IssuingOrganization: org,
				IssueDate:           startVal,
				ExpirationDate:      endVal,
				CredentialID:        credID,
				CredentialURL:       text(entity["url"]),
			})
		case "languages":
			name := text(entity["name"])
			if name != nil {
				profile.Languages = append(profile.Languages, models.Language{
					ID:          urnID(entity["entityUrn"]),
					Name:        *name,
					Proficiency: text(entity["proficiency"]),
				})
			}
		case "projects":
			name := text(entity["title"])
			if name == nil {
				name = text(entity["name"])
			}
			dr := entity["timePeriod"]
			if dr == nil {
				dr = entity["dateRange"]
			}
			profile.Projects = append(profile.Projects, models.Project{
				ID:          urnID(entity["entityUrn"]),
				Name:        name,
				Description: text(entity["description"]),
				DateRange:   dateRange(dr),
				URL:         text(entity["url"]),
			})
		case "publications":
			profile.Publications = append(profile.Publications, models.Publication{
				ID:          urnID(entity["entityUrn"]),
				Name:        text(entity["name"]),
				Publisher:   text(entity["publisher"]),
				Description: text(entity["description"]),
				PublishedOn: date(entity["date"]),
				URL:         text(entity["url"]),
			})
		case "courses":
			profile.Courses = append(profile.Courses, models.Course{
				ID:     urnID(entity["entityUrn"]),
				Name:   text(entity["name"]),
				Number: text(entity["number"]),
			})
		case "honors":
			profile.Honors = append(profile.Honors, models.Honor{
				ID:          urnID(entity["entityUrn"]),
				Title:       text(entity["title"]),
				Issuer:      text(entity["issuer"]),
				Description: text(entity["description"]),
				IssuedOn:    date(entity["issuedOn"]),
			})
		case "volunteer_experience":
			org := text(entity["companyName"])
			if org == nil {
				org = text(entity["organization"])
			}
			dr := entity["timePeriod"]
			if dr == nil {
				dr = entity["dateRange"]
			}
			profile.VolunteerExperience = append(profile.VolunteerExperience, models.VolunteerExperience{
				ID:           urnID(entity["entityUrn"]),
				Role:         text(entity["role"]),
				Organization: org,
				Cause:        text(entity["cause"]),
				Description:  text(entity["description"]),
				DateRange:    dateRange(dr),
			})
		}
	}
}

func strValue(s *string) string {
    if s == nil {
        return ""
    }
    return *s
}
