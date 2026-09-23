package provider

import "github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"

type BranchUpdateError struct {
	Provider string
	Branch   string
	Problem  string
	Err      error
}

func (e *BranchUpdateError) Error() string { return e.Err.Error() }

func (e *BranchUpdateError) Unwrap() error { return e.Err }

func azureBranchUpdateProblem(results *[]git.GitRefUpdateResult) string {
	if results == nil || len(*results) == 0 {
		return "provider returned no branch update result"
	}

	status := (*results)[0].UpdateStatus
	if status == nil {
		return "provider rejected the branch update"
	}

	switch *status {
	case git.GitRefUpdateStatusValues.RejectedByPolicy:
		return "branch update was rejected by policy"
	case git.GitRefUpdateStatusValues.ForcePushRequired:
		return "branch update requires force-push permission"
	case git.GitRefUpdateStatusValues.WritePermissionRequired:
		return "branch update requires write permission"
	case git.GitRefUpdateStatusValues.CreateBranchPermissionRequired:
		return "branch creation requires permission"
	case git.GitRefUpdateStatusValues.StaleOldObjectId:
		return "branch changed before the update completed"
	case git.GitRefUpdateStatusValues.Locked:
		return "branch is locked"
	default:
		return "provider rejected the branch update"
	}
}
