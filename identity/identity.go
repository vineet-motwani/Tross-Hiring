package identity

import "strings"

func ProfilePublicIdentifiers(value interface{}) []string {
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}

	var identifiers []string
	if direct, ok := m["publicIdentifier"].(string); ok && direct != "" {
		identifiers = append(identifiers, direct)
	}

	if mini, ok := m["miniProfile"].(map[string]interface{}); ok {
		if nested, ok := mini["publicIdentifier"].(string); ok && nested != "" {
			identifiers = append(identifiers, nested)
		}
	}
	return identifiers
}

func HasMalformedPublicIdentifier(value interface{}) bool {
	m, ok := value.(map[string]interface{})
	if !ok {
		return false
	}

	if direct, ok := m["publicIdentifier"]; ok {
		if _, isStr := direct.(string); !isStr {
			return true
		}
	}

	if mini, ok := m["miniProfile"].(map[string]interface{}); ok {
		if nested, ok := mini["publicIdentifier"]; ok {
			if _, isStr := nested.(string); !isStr {
				return true
			}
		}
	}
	return false
}

func ProfileMemberIds(value interface{}) []string {
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}

	var urns []interface{}
	urns = append(urns, m["entityUrn"])

	if mini, ok := m["miniProfile"].(map[string]interface{}); ok {
		urns = append(urns, mini["entityUrn"])
	}

	var memberIds []string
	for _, urn := range urns {
		if strUrn, ok := urn.(string); ok {
			parts := strings.Split(strUrn, ":")
			memberId := parts[len(parts)-1]
			if memberId != "" && !strings.HasPrefix(memberId, "(") {
				memberIds = append(memberIds, memberId)
			}
		}
	}
	return memberIds
}
