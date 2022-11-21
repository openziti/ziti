package test_resources

import (
	"embed"
	"github.com/openziti/fablab/resources"
	"io/fs"
)

//go:embed terraform
var terraformResource embed.FS

func TerraformResources() fs.FS {
	return resources.SubFolder(terraformResource, resources.Terraform)
}
