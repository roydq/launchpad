package domain

type AppStatus string

const (
	AppStatusCreated     AppStatus = "created"
	AppStatusDeploying   AppStatus = "deploying"
	AppStatusRunning     AppStatus = "running"
	AppStatusFailed      AppStatus = "failed"
	AppStatusMaintenance AppStatus = "maintenance"
)

type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusLeased    JobStatus = "leased"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusDead      JobStatus = "dead"
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