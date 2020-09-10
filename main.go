package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"text/tabwriter"
	"text/template"

	"github.com/inconshreveable/log15"
	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var (
	destinationFile string
	templateFile    string
	componentsFile  string

	printHelp    bool
	printVersion bool
)

func init() {
	flag.StringVarP(&destinationFile, "output", "o", "", "(required) dhall output file")
	flag.StringVarP(&templateFile, "template", "t", "", "(required) dhall template file to use")
	flag.StringVarP(&componentsFile, "components", "c", "", "(required) components yaml file")
	flag.BoolVarP(&printHelp, "help", "h", false, "print usage instructions")
	flag.BoolVar(&printVersion, "version", false, "print version information")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of ds-to-dhall: --output <output> <path>...\n")
		fmt.Fprintln(os.Stderr, "OPTIONS:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, usageArgs())
	}
}

func usageArgs() string {
	b := bytes.Buffer{}
	w := tabwriter.NewWriter(&b, 0, 8, 1, ' ', 0)

	w.Flush()

	return fmt.Sprintf("ARGS:\n%s", b.String())
}

func versionString(version, commit, date string) string {
	b := bytes.Buffer{}
	w := tabwriter.NewWriter(&b, 0, 8, 1, ' ', 0)

	fmt.Fprintf(w, "version:\t%s", version)
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "commit:\t%s", commit)
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "build date:\t%s", date)
	w.Flush()

	return b.String()
}

func logFatal(message string, ctx ...interface{}) {
	log15.Error(message, ctx...)
	os.Exit(1)
}

func loadComponents(filename string) (map[string]interface{}, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	decoder := yaml.NewDecoder(br)

	comps := make(map[string]interface{})
	err = decoder.Decode(&comps)
	if err != nil {
		return nil, fmt.Errorf("failed to decode yaml file: %s: %v", filename, err)
	}
	return comps, nil
}

func executeTemplate(tmpl *template.Template, data interface{}, outfilename string) error {
	out, err := os.Create(outfilename)
	if err != nil {
		return err
	}
	defer out.Close()

	bout := bufio.NewWriter(out)
	defer bout.Flush()

	return tmpl.Execute(bout, data)
}

type ContainerTuple struct {
	Component     string
	Name          string
	ContainerName string
	Identifier    string
}


type TemplateData struct {
	DeploymentTuples []*ContainerTuple
	StatefulSetTuples []*ContainerTuple
}

/*
Frontend:
    Deployment:
        sourcegraph-frontend:
            containers:
                frontend: {}
                jaeger-agent: {}
    Ingress:
        sourcegraph-frontend: {}
    Role:
        sourcegraph-frontend: {}
    RoleBinding:
        sourcegraph-frontend: {}
    Service:
        sourcegraph-frontend: {}
        sourcegraph-frontend-internal: {}
    ServiceAccount:
        sourcegraph-frontend: {}
Indexed-Search:
    Service:
        indexed-search: {}
        indexed-search-indexer: {}
    StatefulSet:
        indexed-search:
            containers:
                zoekt-indexserver: {}
                zoekt-webserver: {}
 */

func containerTuples(targetKind string, comps map[string]interface{}) []*ContainerTuple {
	var result []*ContainerTuple

	for comp, compData := range comps {
		compDataM, ok := compData.(map[string]interface{})
		if !ok {
			continue
		}

		for kind, kindData := range compDataM {
			if kind != targetKind {
				continue
			}
			kindDataM, ok := kindData.(map[string]interface{})
			if ! ok {
				continue
			}

			for name, nameData := range kindDataM {
				nameDataM, ok := nameData.(map[string]interface{})
				if !ok {
					continue
				}

				for section, sectionData := range nameDataM {
					if section != "containers" {
						continue
					}

					sectionDataM, ok := sectionData.(map[string]interface{})
					if !ok {
						continue
					}

					for containerName := range sectionDataM {
						result = append(result, &ContainerTuple{
							Component: comp,
							Name: name,
							ContainerName: containerName,
							Identifier: fmt.Sprintf("f%d", len(result)),
						})
					}
				}
			}
		}
	}
	return result
}

func main() {
	log15.Root().SetHandler(log15.StreamHandler(os.Stdout, log15.LogfmtFormat()))

	flag.Parse()

	if printHelp {
		flag.Usage()
		os.Exit(0)
	}

	if printVersion {
		output := versionString(version, commit, date)
		fmt.Fprintln(os.Stderr, output)
		os.Exit(0)
	}

	if destinationFile == "" || templateFile == "" || componentsFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	tmpl, err := template.ParseFiles(templateFile)
	if err != nil {
		logFatal("failed to parse template file", "template", templateFile, "error", err)
	}

	comps, err := loadComponents(componentsFile)
	if err != nil {
		logFatal("failed to load components", "components", componentsFile, "error", err)
	}

	data := &TemplateData{
		DeploymentTuples: containerTuples("Deployment", comps),
		StatefulSetTuples: containerTuples("StatefulSet", comps),
	}

	err = executeTemplate(tmpl, data, destinationFile)
	if err != nil {
		logFatal("failed to write template", "out", destinationFile, "error", err)
	}

	log15.Info("Done")
}
