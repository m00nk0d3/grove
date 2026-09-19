package sandcastle

import (
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
)

// Snapshot is the normalized Sandcastle state consumed by mission control.
// Adapter-specific data will be added when the Sandcastle adapter is implemented.
type Snapshot struct {
	Integration domain.ExternalIntegration
	Workflows   []domain.WorkflowRunRef
	Agents      []domain.AgentRef
	CapturedAt  time.Time
}
