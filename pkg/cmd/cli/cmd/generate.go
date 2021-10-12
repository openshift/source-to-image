package cmd

import (
	"context"
	"strings"

	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/spf13/cobra"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/build/strategies/dockerfile"
	"github.com/openshift/source-to-image/pkg/util"
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
func adjustConfigWithImageLabels(cfg *api.Config) {
	cfg.ScriptsURL, cfg.Description = util.AdjustConfigWithImageLabels(cfg)
}

// CanonizeBuilderImageArg appends 'docker://' if the builder image doesn't contain the a schema.
func CanonizeBuilderImageArg(builderImage string) string {
	if strings.Contains(builderImage, "://") {
		return builderImage
	}
	return "docker://" + builderImage
}

// NewCmdGenerate implements the S2I cli generate command.
func NewCmdGenerate(cfg *api.Config) *cobra.Command {
	generateCmd := &cobra.Command{
		Use: "generate <builder image> <output file>",
		Short: "Generate a Dockerfile using an existing S2I builder	image " +
			"that can be used to produce an image by any application " +
			"supporting the format.",
		Example: `
# Generate a Dockerfile for the centos/nodejs-10-centos7 builder image:
$ s2i generate docker.io/centos/nodejs-10-centos7 Dockerfile.gen
`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().NArg() != 2 {
				return cmd.Help()
			}

			cfg.BuilderImage = cmd.Flags().Arg(0)
			cfg.AsDockerfile = cmd.Flags().Arg(1)
			ctx := context.Background()

			err := manageConfigImageLabelsBuildImageName(ctx, cfg)
			if err != nil {
				log.Warningf("could not inspect the builder image for labels: %s", err.Error())
			}
			// for generate we go ahead and modify the config scripts url and destination field since we do not have specific arg
			// overrides for those 2 fields
			adjustConfigWithImageLabels(cfg)
			return generateDockerfile(cfg)
		},
	}

	generateCmd.Flags().BoolVarP(&(cfg.Quiet), "quiet", "q", false, "Operate quietly. Suppress all non-error output.")
	generateCmd.Flags().VarP(&(cfg.Environment), "env", "e", "Specify an single environment variable in NAME=VALUE format")
	generateCmd.Flags().StringVarP(&(cfg.AssembleUser), "assemble-user", "", "", "Specify the user to run assemble with")
	generateCmd.Flags().StringVarP(&(cfg.AssembleRuntimeUser), "assemble-runtime-user", "", "", "Specify the user to run assemble-runtime with")

	return generateCmd
}

// manageConfigImageLabelsBuildImageName extracts the image labels from builder image in the provided config.
// Returns an error if the builder image name is invalid or there is an error extracting the image labels.
func manageConfigImageLabelsBuildImageName(ctx context.Context, cfg *api.Config) error {
	builderImageName := CanonizeBuilderImageArg(cfg.BuilderImage)
	ref, err := alltransports.ParseImageName(builderImageName)
	if err != nil {
		return err
	}

	cfg.BuilderImage = ref.DockerReference().Name()

	if cfg.BuilderImageLabels, err = getImageLabels(ctx, ref); err != nil {
		return err
	}
	return nil
}
