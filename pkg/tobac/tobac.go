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

const SuccessUserIsClusterAdmin = "user is cluster administrator through group '%s'"
const SuccessUserBelongsToTeam = "user belongs to owner team '%s'"
const SuccessUserMatchesServiceUserTemplate = "user matches service user template"
const SuccessUserMayAnnexateOrphanResource = "resource did not have a team label set"

// KubernetesResource represents any Kubernetes resource with standard object metadata structures.
type KubernetesResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
}

type Request struct {
	UserInfo             authenticationv1.UserInfo
	ExistingResource     metav1.Object
	SubmittedResource    metav1.Object
	ClusterAdmins        []string
	ServiceUserTemplates []string
	TeamProvider         TeamProvider
}

type Response struct {
	Allowed bool
	Reason  string
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
		allowedUser := fmt.Sprintf(template, teamID, teamID)
		if username == allowedUser {
			return true
		}
	}
	return false
}

func ClusterAdminResponse(request Request) *Response {
	for _, userGroup := range request.UserInfo.Groups {
		for _, adminGroup := range request.ClusterAdmins {
			if userGroup == adminGroup {
				return &Response{Allowed: true, Reason: fmt.Sprintf(SuccessUserIsClusterAdmin, adminGroup)}
			}
		}
	}

	return nil
}

func Allowed(request Request) Response {
	var team azure.Team
	var teamID string
	var existingLabel string

	// Allow if user is a cluster administrator
	if response := ClusterAdminResponse(request); response != nil {
		return *response
	}

	if request.SubmittedResource != nil {
		// Deny if object is not tagged with a team label.
		teamID = request.SubmittedResource.GetLabels()["team"]
		if len(teamID) == 0 {
			return Response{Allowed: false, Reason: ErrorNotTaggedWithTeamLabel}
		}

		// Deny if specified team does not exist
		team = request.TeamProvider(teamID)
		if !team.Valid() {
			return Response{Allowed: false, Reason: fmt.Sprintf(ErrorTeamDoesNotExistInAzureAD, teamID)}
		}
	}

	// This is an update situation. We must check if the user has access to modify the original resource.
	if request.ExistingResource != nil {
		existingLabel = request.ExistingResource.GetLabels()["team"]

		// If the existing resource does not have a team label, skip permission checks.
		if len(existingLabel) > 0 {

			// Deny if existing team does not exist.
			existingTeam := request.TeamProvider(existingLabel)
			if !existingTeam.Valid() {
				return Response{Allowed: false, Reason: fmt.Sprintf(ErrorExistingTeamDoesNotExistInAzureAD, existingLabel)}
			}

			// If user doesn't belong to the correct team, nor is in the service account access list, deny access.
			serviceUserAccess := hasServiceUserAccess(request.UserInfo.Username, existingTeam.ID, request.ServiceUserTemplates)
			if !stringInSlice(request.UserInfo.Groups, existingTeam.AzureUUID) && !serviceUserAccess {
				return Response{Allowed: false, Reason: fmt.Sprintf(ErrorUserHasNoAccessToTeam, request.UserInfo.Username, existingTeam.ID)}
			}

			// Allow deletes here, since there is no new resource to check
			if request.SubmittedResource == nil {
				if serviceUserAccess {
					return Response{Allowed: true, Reason: SuccessUserMatchesServiceUserTemplate}
				}
				return Response{Allowed: true, Reason: fmt.Sprintf(SuccessUserBelongsToTeam, existingLabel)}
			}
		}

		// Allow deletes here, since there is no new resource to check
		if request.SubmittedResource == nil {
			return Response{Allowed: true, Reason: SuccessUserMayAnnexateOrphanResource}
		}
	}

	// Finally, allow if user exists in the specified team
	if stringInSlice(request.UserInfo.Groups, team.AzureUUID) {
		if request.ExistingResource != nil && len(existingLabel) == 0 {
			return Response{Allowed: true, Reason: SuccessUserMayAnnexateOrphanResource}
		}
		return Response{Allowed: true, Reason: fmt.Sprintf(SuccessUserBelongsToTeam, team.ID)}
	}

	// If user does not exist in the specified team, try to match against service user templates.
	if hasServiceUserAccess(request.UserInfo.Username, team.ID, request.ServiceUserTemplates) {
		return Response{Allowed: true, Reason: SuccessUserMatchesServiceUserTemplate}
	}

	// default deny
	return Response{Allowed: false, Reason: fmt.Sprintf(ErrorUserHasNoAccessToTeam, request.UserInfo.Username, teamID)}
}
