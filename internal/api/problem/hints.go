package problem

import (
	"errors"
	"net/http"
	"strings"

	"github.com/launchpad/launchpad/pkg/launchpad"
)

// HintsFor returns a stable error code and recovery hints for a known failure.
// Specific phrase matches win; sentinel fallbacks use errors.Is.
func HintsFor(err error) (code string, hints []Hint) {
	if err == nil {
		return "", nil
	}
	// Specific phrases first (may also match generic words).
	if code, hints := matchSpecificPhrases(err.Error()); code != "" {
		return code, hints
	}
	// Sentinel fallbacks via errors.Is (stable codes).
	switch {
	case errors.Is(err, launchpad.ErrNotFound):
		return "not_found", []Hint{
			{Action: "doctor", Message: "Verify API, token, project, and environment context.", Command: "launchpad doctor"},
			{Action: "list_projects", Message: "Confirm the project name exists.", Command: "launchpad projects list"},
		}
	case errors.Is(err, launchpad.ErrConflict):
		return "conflict", []Hint{
			{Action: "inspect", Message: "Inspect project state and retry after resolving the conflict.", Command: "launchpad inspect"},
		}
	case errors.Is(err, launchpad.ErrBadRequest):
		return "bad_request", nil
	default:
		return "", nil
	}
}

func matchSpecificPhrases(detail string) (code string, hints []Hint) {
	d := strings.ToLower(detail)
	switch {
	case strings.Contains(d, "changeset is pinned to environment"):
		return "changeset_env_mismatch", []Hint{
			{Action: "review", Message: "Pending changes are pinned to a different environment.", Command: "launchpad diff"},
			{Action: "deploy", Message: "Deploy the pinned batch in its environment, or discard it.", Command: "launchpad deploy"},
			{Action: "reset", Message: "Discard pending changes, then retry in the current environment.", Command: "launchpad reset"},
		}
	case strings.Contains(d, "changeset is empty"):
		return "changeset_empty", []Hint{
			{Action: "stage", Message: "Stage config, image, or scale changes before deploy/push.", Command: "launchpad config set KEY=value"},
			{Action: "deploy_image", Message: "Or deploy an image directly.", Command: "launchpad deploy --image <ref>"},
		}
	case strings.Contains(d, "deployment already in progress"):
		return "deployment_in_progress", []Hint{
			{Action: "wait", Message: "A deploy is already running for this service and environment.", Command: "launchpad deploy --wait"},
			{Action: "inspect", Message: "Check project and job status.", Command: "launchpad inspect"},
		}
	case strings.Contains(d, "artifact is required"):
		return "artifact_required", []Hint{
			{Action: "set_image", Message: "Provide an image for the first release.", Command: "launchpad deploy --image <ref>"},
		}
	case strings.Contains(d, "from and to environments must differ"):
		return "promote_same_env", []Hint{
			{Action: "set_from_to", Message: "Promote requires distinct source and target environments.", Command: "launchpad promote --from staging --to production"},
		}
	case strings.Contains(d, "is not succeeded") || strings.Contains(d, "never successfully deployed"):
		return "promote_invalid_source", []Hint{
			{Action: "list_releases", Message: "Source release must be succeeded and applied in --from.", Command: "launchpad releases"},
			{Action: "deploy_source", Message: "Deploy successfully in the source environment first.", Command: "launchpad env use <from> && launchpad deploy --image <ref> --wait"},
		}
	case strings.Contains(d, "no running release in"):
		return "promote_no_running", []Hint{
			{Action: "pass_version", Message: "No running deploy in source; pass --release explicitly.", Command: "launchpad promote --from <env> --release <n>"},
		}
	case strings.Contains(d, "layer must be"):
		return "config_layer_invalid", []Hint{
			{Action: "use_layer", Message: "Use layer shared, service, or resolved.", Command: "launchpad config get --layer shared"},
		}
	case strings.Contains(d, "unknown manifest field"),
		strings.Contains(d, "empty manifest"),
		strings.Contains(d, "must be a scalar string"),
		strings.Contains(d, "must not be null"),
		strings.Contains(d, "manifest environments is required"),
		strings.Contains(d, "invalid project name"),
		strings.Contains(d, "invalid process name"),
		strings.Contains(d, "invalid environment name"),
		strings.Contains(d, "invalid process expose"):
		return "manifest_invalid", []Hint{
			{Action: "export", Message: "v1 launchpad.yaml: version, project, processes, environments. Unknown keys are rejected.", Command: "launchpad export"},
		}
	case strings.Contains(d, "unsupported manifest version"):
		return "manifest_version", []Hint{
			{Action: "set_version", Message: "launchpad.yaml v1 uses version: 1.", Command: "launchpad export"},
		}
	case strings.Contains(d, "manifest must not contain a secret value"):
		return "manifest_secret_value", []Hint{
			{Action: "secret_keys", Message: "List secret names under config.secret_keys; never put values in the file.", Command: "launchpad config set --secret KEY=value"},
		}
	case strings.Contains(d, "manifest field is deferred"):
		return "manifest_deferred", []Hint{
			{Action: "single_service", Message: "v1 manifests describe one primary service; multi-service and bindings are deferred.", Command: "launchpad inspect"},
		}
	case strings.Contains(d, "selected environment") && strings.Contains(d, "manifest"):
		return "manifest_env", []Hint{
			{Action: "add_env", Message: "The selected environment must exist in the document.", Command: "launchpad env list"},
		}
	case strings.Contains(d, "creating a project requires environments.dev"):
		return "manifest_env", []Hint{
			{Action: "add_dev", Message: "Creating a project via apply requires environments.dev (bootstrap).", Command: "launchpad apply -f launchpad.yaml --env dev"},
		}
	case strings.Contains(d, "project name does not match"):
		return "manifest_project_mismatch", []Hint{
			{Action: "rename", Message: "document.project must equal the URL project name.", Command: "launchpad apply -f launchpad.yaml"},
		}
	default:
		return "", nil
	}
}

// hintsForDetail is used by legacy helpers that only have a detail string (no error).
func hintsForDetail(detail string) (code string, hints []Hint) {
	if code, hints := matchSpecificPhrases(detail); code != "" {
		return code, hints
	}
	d := strings.ToLower(detail)
	switch {
	case strings.Contains(d, "not found"):
		return "not_found", []Hint{
			{Action: "doctor", Message: "Verify API, token, project, and environment context.", Command: "launchpad doctor"},
			{Action: "list_projects", Message: "Confirm the project name exists.", Command: "launchpad projects list"},
		}
	case strings.Contains(d, "conflict"):
		return "conflict", []Hint{
			{Action: "inspect", Message: "Inspect project state and retry after resolving the conflict.", Command: "launchpad inspect"},
		}
	case strings.Contains(d, "bad request"):
		return "bad_request", nil
	default:
		return "", nil
	}
}

func statusTitle(err error) (int, string) {
	switch {
	case errors.Is(err, launchpad.ErrNotFound):
		return http.StatusNotFound, "Not Found"
	case errors.Is(err, launchpad.ErrConflict):
		return http.StatusConflict, "Conflict"
	case errors.Is(err, launchpad.ErrBadRequest):
		return http.StatusBadRequest, "Bad Request"
	case errors.Is(err, launchpad.ErrNotImplemented):
		return http.StatusNotImplemented, "Not Implemented"
	case errors.Is(err, launchpad.ErrUnauthorized):
		return http.StatusUnauthorized, "Unauthorized"
	case errors.Is(err, launchpad.ErrForbidden):
		return http.StatusForbidden, "Forbidden"
	default:
		return http.StatusInternalServerError, "Internal Server Error"
	}
}

func typeURI(status int) string {
	slug := statusSlug(status)
	return "https://launchpad.dev/errors/" + slug
}

func statusSlug(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad-request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not-found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusInternalServerError:
		return "internal"
	case http.StatusNotImplemented:
		return "not-implemented"
	default:
		return strings.ToLower(strings.ReplaceAll(http.StatusText(status), " ", "-"))
	}
}
