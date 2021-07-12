package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v2"
)

type appConfig struct {
	Host string `yaml:"host"`
}

type configSettings struct {
	Name string `yaml:"name"`
	File string `yaml:"file"`
}

type composeInfo struct {
	Configs map[string]configSettings `yaml:"configs"`
	Secrets map[string]configSettings `yaml:"secrets"`
}

func fileHash(filePath string) (string, error) {
	var fileHash string

	file, err := os.Open(filePath)
	if err != nil {
		return fileHash, err
	}

	defer func() { _ = file.Close() }()

	hash := sha256.New()

	if _, err := io.Copy(hash, file); err != nil {
		return fileHash, err
	}

	hashBytes := hash.Sum(nil)[:8]
	fileHash = hex.EncodeToString(hashBytes)

	return fileHash, nil
}

func newFileEnvironment(filePath string) (string, error) {
	variable := strings.ToUpper(path.Base(filePath))

	re := regexp.MustCompile(`[^A-Z0-9_]`)
	variable = re.ReplaceAllString(variable, "_")

	version, err := fileHash(filePath)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s=%s", variable, version), nil
}

func environmentFromYaml(yamlFile []byte) ([]string, error) {
	var environment []string
	var cfg composeInfo

	err := yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		return environment, err
	}

	for _, v := range cfg.Configs {
		if v.Name != "" {
			env, err := newFileEnvironment(v.File)
			if err != nil {
				log.Printf("Cannot generate environment for config file %s: %s", v.File, err.Error())
			} else {
				log.Printf("Using config %s\n", env)
				environment = append(environment, env)
			}
		}
	}

	for _, v := range cfg.Secrets {
		if v.Name != "" {
			env, err := newFileEnvironment(v.File)
			if err != nil {
				log.Printf("Cannot generate environment for secret file %s: %s", v.File, err.Error())
			} else {
				log.Printf("Using secret %s\n", env)
				environment = append(environment, env)
			}
		}
	}

	return environment, nil
}

func loadEnvFromConfigFiles(filenames []string, stdin io.Reader) ([]string, error) {
	var envs []string

	for _, filename := range filenames {
		env, err := loadEnvFromConfigFile(filename, stdin)
		if err != nil {
			return envs, err
		}
		envs = append(envs, env...)
	}

	return envs, nil
}

func loadEnvFromConfigFile(filename string, stdin io.Reader) ([]string, error) {
	var yamlFile []byte
	var err error

	if filename == "-" {
		yamlFile, err = ioutil.ReadAll(stdin)
	} else {
		yamlFile, err = ioutil.ReadFile(filename)
	}

	if err != nil {
		return nil, err
	}

	return environmentFromYaml(yamlFile)
}

func loadAppConfig(filename string) (*appConfig, error) {
	cfg := &appConfig{}
	targetPath, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	rootDir := filepath.Join(filepath.VolumeName(targetPath), "/")

	for {
		configPath := filepath.Join(targetPath, filename)
		data, err := ioutil.ReadFile(configPath)
		if err != nil {
			if os.IsNotExist(err) {
				if targetPath == rootDir {
					return cfg, nil
				}
				targetPath = filepath.Dir(targetPath)
				continue
			}
			break
		}

		log.Printf("Reading config file: %s", configPath)
		err = yaml.Unmarshal(data, cfg)
		if err != nil {
			break
		}

		return cfg, nil
	}

	return nil, err
}

var auth = flag.BoolP("with-registry-auth", "a", false, "Send registry authentication details to Swarm agents")
var prune = flag.BoolP("prune", "p", false, "Prune services that are no longer referenced")
var host = flag.StringP("host", "H", "", "Daemon socket(s) to connect to")
var composeFiles = flag.StringSliceP("compose-file", "c", []string{"docker-compose.yml"}, "Path to a Compose file, or '-' to read from stdin")

func main() {
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	err := flag.CommandLine.Parse(os.Args[1:])
	log.SetFlags(0)

	if err == flag.ErrHelp {
		os.Exit(0)
	} else if err != nil {
		log.Fatal(err)
	}

	cfg, err := loadAppConfig(".docker-deploy.yml")
	if err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer
	tee := io.TeeReader(os.Stdin, &buf)
	env, err := loadEnvFromConfigFiles(*composeFiles, tee)
	if err != nil {
		log.Fatal(err)
	}

	args := []string{"stack", "deploy"}
	for _, composeFile := range *composeFiles {
		args = append(args, "--compose-file", composeFile)
	}

	if *host != "" {
		args = append([]string{"--host", *host}, args...)
	} else if cfg.Host != "" {
		args = append([]string{"--host", cfg.Host}, args...)
	}

	if *auth {
		args = append(args, "--with-registry-auth")
	}

	if *prune {
		args = append(args, "--prune")
	}

	if len(flag.Args()) == 0 {
		dirname, err := os.Getwd()
		if err != nil {
			log.Fatalf("No stack name provided and cannot read the current directory: %s", err.Error())
		}

		args = append(args, filepath.Base(dirname))
	} else {
		args = append(args, flag.Args()...)
	}

	log.Printf("Running: docker %v", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if buf.Len() > 0 {
		cmd.Stdin = &buf
	} else {
		cmd.Stdin = os.Stdin
	}

	err = cmd.Run()
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			os.Exit(exiterr.ExitCode())
		} else {
			log.Fatal(err)
		}
	}
}
