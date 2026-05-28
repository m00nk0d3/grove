package version

// Version is set at build time via:
//
//	go build -ldflags "-X github.com/m00nk0d3/grove/internal/version.Version=vX.Y.Z" ./...
var Version = "dev"
