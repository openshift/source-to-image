package cmd

import (
	"context"

	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/spf13/cobra"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/constants"
	"github.com/openshift/source-to-image/pkg/build/strategies/dockerfile"
	"github.com/openshift/source-to-image/pkg/util/fs"
)

// getImageLabels attempts to inspect an image existing in a remote registry.
func getImageLabels(ctx context.Context, ref types.ImageReference) (map[string]string, error) {
	img, err := ref.NewImage(ctx, &types.SystemContext{})
	if err != nil {
		return nil, err
	}

	imageMetadata, err := img.Inspect(ctx)
	if err != nil {
		return nil, err
	}

	return imageMetadata.Labels, nil
}

// generateDockerfile generates a Dockerfile with the given configuration.
func generateDockerfile(cfg *api.Config) error {
	fileSystem := fs.NewFileSystem()
	builder, err := dockerfile.New(cfg, fileSystem)
	if err != nil {
		return err
	}

	_, err = builder.Build(cfg)
	if err != nil {
		return err
	}

	return nil
}

// adjustConfigWithImageLabels adjusts the configuration with given labels.
func adjustConfigWithImageLabels(cfg *api.Config, labels map[string]string) {
	if v, ok := labels[constants.ScriptsURLLabel]; ok {
		cfg.ScriptsURL = v
	}

	if v, ok := labels[constants.DestinationLabel]; ok {
		cfg.Destination = v
	}

}

// NewCmdGenerate implements the S2I cli generate command.
func NewCmdGenerate(cfg *api.Config) *cobra.Command {
	generateCmd := &cobra.Command{
		Use:   "generate <image> <dockerfile>",
		Short: "Generate a Dockerfile based on the provided builder image",
		Example: `
# Generate a Dockerfile from a builder image:
$ s2i generate docker.io/centos/nodejs-10-centos7 Dockerfile.gen
`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().NArg() != 2 {
				return cmd.Help()
			}

			cfg.BuilderImage = cmd.Flags().Arg(0)
			cfg.AsDockerfile = cmd.Flags().Arg(1)

			ref, err := alltransports.ParseImageName("docker://" + cfg.BuilderImage)
			if err != nil {
				return err
			}

			ctx := context.Background()
			var imageLabels map[string]string
			if imageLabels, err = getImageLabels(ctx, ref); err != nil {
				return err
			}

			adjustConfigWithImageLabels(cfg, imageLabels)
			return generateDockerfile(cfg)
		},
	}

	generateCmd.Flags().BoolVarP(&(cfg.Quiet), "quiet", "q", false, "Operate quietly. Suppress all non-error output.")
	generateCmd.Flags().VarP(&(cfg.Environment), "env", "e", "Specify an single environment variable in NAME=VALUE format")
	generateCmd.Flags().StringVarP(&(cfg.AssembleUser), "assemble-user", "", "", "Specify the user to run assemble with")
	generateCmd.Flags().StringVarP(&(cfg.AssembleRuntimeUser), "assemble-runtime-user", "", "", "Specify the user to run assemble-runtime with")

	return generateCmd
}
