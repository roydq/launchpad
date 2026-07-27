package domain

type ProjectStatus string

const (
	ProjectStatusCreated   ProjectStatus = "created"
	ProjectStatusDeploying ProjectStatus = "deploying"
	ProjectStatusRunning   ProjectStatus = "running"
	ProjectStatusFailed    ProjectStatus = "failed"
)

type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusLeased    JobStatus = "leased"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
)

type ReleaseStatus string

const (
	ReleaseStatusPending   ReleaseStatus = "pending"
	ReleaseStatusSucceeded ReleaseStatus = "succeeded"
	ReleaseStatusFailed    ReleaseStatus = "failed"
)

type DeploymentStatus string

const (
	DeploymentPending    DeploymentStatus = "pending"
	DeploymentDeploying  DeploymentStatus = "deploying"
	DeploymentRunning    DeploymentStatus = "running"
	DeploymentFailed     DeploymentStatus = "failed"
	DeploymentCancelled  DeploymentStatus = "cancelled"
	DeploymentSuperseded DeploymentStatus = "superseded"
)