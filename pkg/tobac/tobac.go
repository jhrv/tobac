package tobac

import (
	"fmt"

	"github.com/nais/tobac/pkg/azure"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ErrorNotTaggedWithTeamLabel = "object is not tagged with a team label"
const ErrorTeamDoesNotExistInAzureAD = "team '%s' does not exist in Azure AD"
const ErrorExistingTeamDoesNotExistInAzureAD = "team '%s' on existing resource does not exist in Azure AD"
const ErrorUserHasNoAccessToTeam = "user '%s' has no access to team '%s'"

// KubernetesResource represents any Kubernetes resource with standard object metadata structures.
type KubernetesResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
}

type Request struct {
	UserInfo             authenticationv1.UserInfo
	ExistingResource     *KubernetesResource
	SubmittedResource    *KubernetesResource
	ClusterAdmins        []string
	ServiceUserTemplates []string
	TeamProvider         TeamProvider
}

type TeamProvider func(string) azure.Team

func stringInSlice(slice []string, str string) bool {
	for _, s := range slice {
		if str == s {
			return true
		}
	}
	return false
}

// Check if a user is in the service user access list.
func hasServiceUserAccess(username, teamID string, templates []string) bool {
	for _, template := range templates {
		allowedUser := fmt.Sprintf(template, teamID)
		if username == allowedUser {
			return true
		}
	}
	return false
}

func Allowed(request Request) error {
	var team azure.Team
	var teamID string

	// Allow if user is a cluster administrator
	for _, userGroup := range request.UserInfo.Groups {
		for _, adminGroup := range request.ClusterAdmins {
			if userGroup == adminGroup {
				return nil
			}
		}
	}

	if request.SubmittedResource != nil {
		// Deny if object is not tagged with a team label.
		teamID = request.SubmittedResource.Labels["team"]
		if len(teamID) == 0 {
			return fmt.Errorf(ErrorNotTaggedWithTeamLabel)
		}

		// Deny if specified team does not exist
		team = request.TeamProvider(request.SubmittedResource.Labels["team"])
		if !team.Valid() {
			return fmt.Errorf(ErrorTeamDoesNotExistInAzureAD, request.SubmittedResource.Labels["team"])
		}
	}

	// This is an update situation. We must check if the user has access to modify the original resource.
	if request.ExistingResource != nil {
		label := request.ExistingResource.Labels["team"]

		// If the existing resource does not have a team label, skip permission checks.
		if len(label) > 0 {

			// Deny if existing team does not exist.
			existingTeam := request.TeamProvider(label)
			if !existingTeam.Valid() {
				return fmt.Errorf(ErrorExistingTeamDoesNotExistInAzureAD, label)
			}

			// If user doesn't belong to the correct team, nor is in the service account access list, deny access.
			if !stringInSlice(request.UserInfo.Groups, existingTeam.AzureUUID) && !hasServiceUserAccess(request.UserInfo.Username, existingTeam.ID, request.ServiceUserTemplates) {
				return fmt.Errorf(ErrorUserHasNoAccessToTeam, request.UserInfo.Username, existingTeam.ID)
			}
		}

		if request.SubmittedResource == nil {
			return nil
		}
	}

	// Finally, allow if user exists in the specified team
	if stringInSlice(request.UserInfo.Groups, team.AzureUUID) {
		return nil
	}

	// If user does not exist in the specified team, try to match against service user templates.
	if hasServiceUserAccess(request.UserInfo.Username, team.ID, request.ServiceUserTemplates) {
		return nil
	}

	// default deny
	return fmt.Errorf(ErrorUserHasNoAccessToTeam, request.UserInfo.Username, teamID)
}
