package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	componentsFile string

	printHelp    bool
	printVersion bool
)

func init() {
	flag.StringVarP(&componentsFile, "components", "c", "", "(required) components yaml file")
	flag.BoolVarP(&printHelp, "help", "h", false, "print usage instructions")
	flag.BoolVar(&printVersion, "version", false, "print version information")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of dhallie: \n")
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

type KindTuple struct {
	Component  string
	Name       string
	Kind       string
	Identifier string
}

type TemplateData struct {
	DeploymentTuples  []*ContainerTuple
	StatefulSetTuples []*ContainerTuple
	KindTuples        []*KindTuple
}

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
			if !ok {
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
							Component:     comp,
							Name:          name,
							ContainerName: containerName,
							Identifier:    fmt.Sprintf("f%d", len(result)),
						})
					}
				}
			}
		}
	}
	return result
}

func kindTuples(comps map[string]interface{}) []*KindTuple {
	var result []*KindTuple

	for comp, compData := range comps {
		compDataM, ok := compData.(map[string]interface{})
		if !ok {
			continue
		}

		for kind, kindData := range compDataM {
			kindDataM, ok := kindData.(map[string]interface{})
			if !ok {
				continue
			}

			for name := range kindDataM {
				result = append(result, &KindTuple{
					Component:  comp,
					Kind:       kind,
					Name:       name,
					Identifier: fmt.Sprintf("f%d", len(result)),
				})
			}
		}
	}
	return result
}

func dhallFormat(file string) error {
	cmd := exec.Command("dhall", "format", "--inplace", file)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func processTemplate(templatePath string, data *TemplateData) error {
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("failed to parse template file %s: %v", templatePath, err)
	}

	outPath := strings.TrimSuffix(templatePath, filepath.Ext(templatePath))
	outPath = outPath + ".dhall"

	err = executeTemplate(tmpl, data, outPath)
	if err != nil {
		return fmt.Errorf("failed to write output %s: %v", outPath, err)
	}

	err = dhallFormat(outPath)
	if err != nil {
		return fmt.Errorf("failed to format output %s: %v", outPath, err)
	}
	return nil
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

	if componentsFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	comps, err := loadComponents(componentsFile)
	if err != nil {
		logFatal("failed to load components", "components", componentsFile, "error", err)
	}

	inputs := flag.Args()
	if len(inputs) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			logFatal("failed to get cwd for inputs", "err", err)
		}
		inputs = []string{cwd}
	}

	data := &TemplateData{
		DeploymentTuples:  containerTuples("Deployment", comps),
		StatefulSetTuples: containerTuples("StatefulSet", comps),
		KindTuples:        kindTuples(comps),
	}

	for _, input := range inputs {
		err = filepath.Walk(input, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if filepath.Ext(path) == ".dhall-template" {
				err = processTemplate(path, data)
				if err != nil {
					return fmt.Errorf("failed to process %s: %v", path, err)
				}
			}
			return nil
		})
		if err != nil {
			logFatal("failed to process inputs", "error", err)
		}
	}

	log15.Info("Done")
}
