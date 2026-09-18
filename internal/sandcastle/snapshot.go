package sandcastle

import "github.com/m00nk0d3/grove/internal/domain"

// Snapshot is the normalized Sandcastle state consumed by mission control.
// Adapter-specific data will be added when the Sandcastle adapter is implemented.
type Snapshot struct {
	Integration domain.ExternalIntegration
}
