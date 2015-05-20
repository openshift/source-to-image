package api

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

func (r *Request) PrintObj() string {
	out, err := tabbedString(func(out io.Writer) error {
		fmt.Fprintf(out, "Builder Image:\t%s\n", r.BaseImage)
		fmt.Fprintf(out, "Source:\t%s\n", r.Source)
		if len(r.Ref) > 0 {
			fmt.Fprintf(out, "Source Ref:\t%s\n", r.Ref)
		}
		if len(r.ContextDir) > 0 {
			fmt.Fprintf(out, "Context Directory:\t%s\n", r.ContextDir)
		}
		fmt.Fprintf(out, "Output Image Tag:\t%s\n", r.Tag)
		if len(r.InstallDestination) > 0 {
			fmt.Fprintf(out, "Install Destination:\t%s\n", r.InstallDestination)
		}
		printEnv(out, r.Environment)
		if len(r.EnvironmentFile) > 0 {
			fmt.Fprintf(out, "Environment File:\t%s\n", r.EnvironmentFile)
		}
		fmt.Fprintf(out, "Incremental Build:\t%s\n", printBool(r.Incremental))
		fmt.Fprintf(out, "Remove Old Build:\t%s\n", printBool(r.RemovePreviousImage))
		fmt.Fprintf(out, "Force Pull:\t%s\n", printBool(r.ForcePull))
		fmt.Fprintf(out, "Quiet:\t%s\n", printBool(r.Quiet))
		fmt.Fprintf(out, "Layered Build:\t%s\n", printBool(r.LayeredBuild))
		if len(r.Location) > 0 {
			fmt.Fprintf(out, "Artifacts Location:\t%s\n", r.Location)
		}
		if len(r.CallbackURL) > 0 {
			fmt.Fprintf(out, "Callback URL:\t%s\n", r.CallbackURL)
		}
		if len(r.ScriptsURL) > 0 {
			fmt.Fprintf(out, "STI Scripts URL:\t%s\n", r.ScriptsURL)
		}
		if len(r.WorkingDir) > 0 {
			fmt.Fprintf(out, "Workdir:\t%s\n", r.WorkingDir)
		}
		fmt.Fprintf(out, "Docker Endpoint:\t%s\n", r.DockerConfig.Endpoint)

		if _, err := os.Open(r.DockerCfgPath); err == nil {
			fmt.Fprintf(out, "Docker Pull Config:\t%s\n", r.DockerCfgPath)
			fmt.Fprintf(out, "Docker Pull User:\t%s\n", r.PullAuthentication.Username)
		}
		return nil
	})
	if err != nil {
		fmt.Printf("ERROR: %v", err)
	}
	return out
}

func printEnv(out io.Writer, env map[string]string) {
	if len(env) == 0 {
		return
	}
	result := []string{}
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	fmt.Fprintf(out, "Environment:\t%s\n", strings.Join(result, ","))
}

func printBool(b bool) string {
	if b {
		return "\033[1menabled\033[0m"
	}
	return "disabled"
}

func tabbedString(f func(io.Writer) error) (string, error) {
	out := new(tabwriter.Writer)
	buf := &bytes.Buffer{}
	out.Init(buf, 0, 8, 1, '\t', 0)

	err := f(out)
	if err != nil {
		return "", err
	}

	out.Flush()
	str := string(buf.String())
	return str, nil
}
