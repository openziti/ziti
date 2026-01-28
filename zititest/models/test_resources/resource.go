package test_resources

import (
	"embed"
	"io/fs"

	"github.com/openziti/fablab/resources"
)

//go:embed terraform
var terraformResource embed.FS

func TerraformResources() fs.FS {
	return resources.SubFolder(terraformResource, resources.Terraform)
}
