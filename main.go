package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jfrog/jfrog-cli/general/ai"
	"github.com/jfrog/jfrog-client-go/http/httpclient"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/agnivade/levenshtein"
	corecommon "github.com/jfrog/jfrog-cli-core/v2/docs/common"
	setupcore "github.com/jfrog/jfrog-cli-core/v2/general/envsetup"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreconfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/log"
	securityCLI "github.com/jfrog/jfrog-cli-security/cli"
	"github.com/jfrog/jfrog-cli/artifactory"
	"github.com/jfrog/jfrog-cli/buildtools"
	"github.com/jfrog/jfrog-cli/completion"
	"github.com/jfrog/jfrog-cli/config"
	"github.com/jfrog/jfrog-cli/distribution"
	"github.com/jfrog/jfrog-cli/docs/common"
	aiDocs "github.com/jfrog/jfrog-cli/docs/general/ai"
	"github.com/jfrog/jfrog-cli/docs/general/cisetup"
	loginDocs "github.com/jfrog/jfrog-cli/docs/general/login"
	tokenDocs "github.com/jfrog/jfrog-cli/docs/general/token"
	cisetupcommand "github.com/jfrog/jfrog-cli/general/cisetup"
	"github.com/jfrog/jfrog-cli/general/envsetup"
	"github.com/jfrog/jfrog-cli/general/login"
	"github.com/jfrog/jfrog-cli/general/project"
	"github.com/jfrog/jfrog-cli/general/token"
	"github.com/jfrog/jfrog-cli/lifecycle"
	"github.com/jfrog/jfrog-cli/missioncontrol"
	"github.com/jfrog/jfrog-cli/pipelines"
	"github.com/jfrog/jfrog-cli/plugins"
	"github.com/jfrog/jfrog-cli/plugins/utils"
	"github.com/jfrog/jfrog-cli/utils/cliutils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	clientlog "github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/urfave/cli"
	"golang.org/x/exp/slices"
)

const commandHelpTemplate string = `{{.HelpName}}{{if .UsageText}}
Arguments:
{{.UsageText}}
{{end}}{{if .VisibleFlags}}
Options:
	{{range .VisibleFlags}}{{.}}
	{{end}}{{end}}{{if .ArgsUsage}}
Environment Variables:
{{.ArgsUsage}}{{end}}

`

const jfrogAppName = "jf"

func main() {
	log.SetDefaultLogger()
	err := execMain()
	if cleanupErr := fileutils.CleanOldDirs(); cleanupErr != nil {
		clientlog.Warn(cleanupErr)
	}
	coreutils.ExitOnErr(err)
}

// Command is a subcommand for a cli.App.
type Command struct {
	// The name of the command
	Name string
	// short name of the command. Typically one character (deprecated, use `Aliases`)
	ShortName string
	// A list of aliases for the command
	Aliases []string
	// A short description of the usage of this command
	Usage string
	// Custom text to show on USAGE section of help
	UsageText string
	// A longer explanation of how the command works
	Description string
	// A short description of the arguments of this command
	ArgsUsage string
	// The category the command is part of
	Category string
	// The function to call when checking for bash command completions
	BashComplete cli.BashCompleteFunc `json:"-"`
	// An action to execute before any sub-subcommands are run, but after the context is ready
	// If a non-nil error is returned, no sub-subcommands are run
	Before cli.BeforeFunc `json:"-"`
	// An action to execute after any subcommands are run, but after the subcommand has finished
	// It is run even if Action() panics
	After cli.AfterFunc `json:"-"`
	// The function to call when this command is invoked
	Action interface{} `json:"-"`
	// TODO: replace `Action: interface{}` with `Action: ActionFunc` once some kind
	// of deprecation period has passed, maybe?

	// Execute this function if a usage error occurs.
	OnUsageError cli.OnUsageErrorFunc `json:"-"`
	// List of child commands
	Subcommands Commands
	// List of flags to parse
	Flags []cli.Flag
	// Treat all flags as normal arguments if true
	SkipFlagParsing bool
	// Skip argument reordering which attempts to move flags before arguments,
	// but only works if all flags appear after all arguments. This behavior was
	// removed n version 2 since it only works under specific conditions so we
	// backport here by exposing it as an option for compatibility.
	SkipArgReorder bool
	// Boolean to hide built-in help command
	HideHelp bool
	// Boolean to hide this command from help or completion
	Hidden bool
	// Boolean to enable short-option handling so user can combine several
	// single-character bool arguments into one
	// i.e. foobar -o -v -> foobar -ov
	UseShortOptionHandling bool

	// Full name of command for help, defaults to full command name, including parent commands.
	HelpName        string
	commandNamePath []string

	// CustomHelpTemplate the text template for the command help topic.
	// cli.go uses text/template to render templates. You can
	// render custom help text by setting this variable.
	CustomHelpTemplate string
}

type MyCommand struct {
	Name        string      `json:"name,omitempty"`
	ShortName   string      `json:"shortName,omitempty"`
	Description string      `json:"description,omitempty"`
	Args        string      `json:"args,omitempty"`
	Usage       string      `json:"usage,omitempty"`
	Subcommands []MyCommand `json:"subcommands,omitempty"`
	Flags       []MyFlag    `json:"flags,omitempty"`
}

type MyFlag struct {
	Name  string `json:"name,omitempty"`
	Usage string `json:"usage,omitempty"`
}

// Copy function for Command
func CopyCommand(com1 cli.Command) MyCommand {
	com2 := MyCommand{}
	com2.Name = com1.Name
	if len(com1.Aliases) > 0 {
		com2.ShortName = com1.Aliases[0]
	}
	com2.Usage = com1.HelpName
	com2.Description = com1.Usage
	com2.Args = com1.UsageText
	for _, flag := range com1.Flags {
		com2.Flags = append(com2.Flags, MyFlag{Name: flag.GetName(), Usage: flag.String()})
	}
	return com2
}

// Commands is a slice of Command
type Commands []Command

func execMain() error {
	// Set JFrog CLI's user-agent on the jfrog-client-go.
	clientutils.SetUserAgent(coreutils.GetCliUserAgent())

	app := cli.NewApp()
	app.Name = jfrogAppName
	app.Usage = "See https://docs.jfrog-applications.jfrog.io/jfrog-applications/jfrog-cli for full documentation."
	app.Version = cliutils.GetVersion()
	args := os.Args
	cliutils.SetCliExecutableName(args[0])
	app.EnableBashCompletion = true
	commands, err := getCommands()

	allCommands := make(map[string][]MyCommand)

	for _, com := range commands {
		if com.Hidden {
			continue
		}
		myCom := CopyCommand(com)
		for _, sub := range com.Subcommands {
			if sub.Hidden {
				continue
			}
			mySub := CopyCommand(sub)
			for _, subsub := range sub.Subcommands {
				if subsub.Hidden {
					continue
				}
				mySubSub := CopyCommand(subsub)
				mySub.Subcommands = append(mySub.Subcommands, mySubSub)
			}
			myCom.Subcommands = append(myCom.Subcommands, mySub)
		}
		if com.Category == "" {
			com.Category = otherCategory
		}
		allCommands[com.Category] = append(allCommands[com.Category], myCom)
	}
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(allCommands)
	if err != nil {
		panic(err)
	}
	jsonStr := buffer.String()
	jsonStr = jsonStr
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
	app.Commands = commands
	cli.CommandHelpTemplate = commandHelpTemplate
	cli.AppHelpTemplate = getAppHelpTemplate()
	app.CommandNotFound = func(c *cli.Context, command string) {
		_, err = fmt.Fprintf(c.App.Writer, "'"+c.App.Name+" "+command+"' is not a jf command. See --help\n")
		if err != nil {
			clientlog.Debug(err)
			os.Exit(1)
		}
		if bestSimilarity := searchSimilarCmds(c.App.Commands, command); len(bestSimilarity) > 0 {
			text := "The most similar "
			if len(bestSimilarity) == 1 {
				text += "command is:\n\tjf " + bestSimilarity[0]
			} else {
				sort.Strings(bestSimilarity)
				text += "commands are:\n\tjf " + strings.Join(bestSimilarity, "\n\tjf ")
			}
			_, err = fmt.Fprintln(c.App.Writer, text)
			if err != nil {
				clientlog.Debug(err)
			}
		}
		os.Exit(1)
	}
	app.Before = func(ctx *cli.Context) error {
		clientlog.Debug("JFrog CLI version:", app.Version)
		clientlog.Debug("OS/Arch:", runtime.GOOS+"/"+runtime.GOARCH)
		warningMessage, err := cliutils.CheckNewCliVersionAvailable(app.Version)
		if err != nil {
			clientlog.Debug("failed while trying to check latest JFrog CLI version:", err.Error())
		}
		if warningMessage != "" {
			clientlog.Warn(warningMessage)
		}
		if err = setUberTraceIdToken(); err != nil {
			clientlog.Warn("failed generating a trace ID token:", err.Error())
		}
		return nil
	}
	err = app.Run(args)
	return err
}

// This command generates and sets an Uber Trace ID token which will be attached as a header to every request.
// This allows users to easily identify which logs on the server side are related to the command executed by the CLI.
func setUberTraceIdToken() error {
	traceID, err := generateTraceIdToken()
	if err != nil {
		return err
	}
	httpclient.SetUberTraceIdToken(traceID)
	clientlog.Debug("Trace ID for JFrog Platform logs: ", traceID)
	return nil
}

// Generates a 16 chars hexadecimal string to be used as a Trace ID token.
func generateTraceIdToken() (string, error) {
	// Generate 8 random bytes.
	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	// Convert the random bytes to a 16 chars hexadecimal string.
	return hex.EncodeToString(buf), nil
}

// Detects typos and can identify one or more valid commands similar to the error command.
// In Addition, if a subcommand is found with exact match, preferred it over similar commands, for example:
// "jf bp" -> return "jf rt bp"
func searchSimilarCmds(cmds []cli.Command, toCompare string) (bestSimilarity []string) {
	// Set min diff between two commands.
	minDistance := 2
	for _, cmd := range cmds {
		// Check if we have an exact match with the next level.
		for _, subCmd := range cmd.Subcommands {
			for _, subCmdName := range subCmd.Names() {
				// Found exact match, return it.
				distance := levenshtein.ComputeDistance(subCmdName, toCompare)
				if distance == 0 {
					return []string{cmd.Name + " " + subCmdName}
				}
			}
		}
		// Search similar commands with max diff of 'minDistance'.
		for _, cmdName := range cmd.Names() {
			distance := levenshtein.ComputeDistance(cmdName, toCompare)
			if distance == minDistance {
				// In the case of an alias, we don't want to show the full command name, but the alias.
				// Therefore, we trim the end of the full name and concat the actual matched (alias/full command name)
				bestSimilarity = append(bestSimilarity, strings.Replace(cmd.FullName(), cmd.Name, cmdName, 1))
			}
			if distance < minDistance {
				// Found a cmd with a smaller distance.
				minDistance = distance
				bestSimilarity = []string{strings.Replace(cmd.FullName(), cmd.Name, cmdName, 1)}
			}
		}
	}
	return
}

const otherCategory = "Other"
const commandNamespacesCategory = "Command Namespaces"

func getCommands() ([]cli.Command, error) {
	cliNameSpaces := []cli.Command{
		{
			Name:        cliutils.CmdArtifactory,
			Usage:       "Artifactory commands.",
			Subcommands: artifactory.GetCommands(),
			Category:    commandNamespacesCategory,
		},
		{
			Name:        cliutils.CmdMissionControl,
			Usage:       "Mission Control commands.",
			Subcommands: missioncontrol.GetCommands(),
			Category:    commandNamespacesCategory,
		},
		{
			Name:        cliutils.CmdDistribution,
			Usage:       "Distribution V1 commands.",
			Subcommands: distribution.GetCommands(),
			Category:    commandNamespacesCategory,
		},
		{
			Name:        cliutils.CmdPipelines,
			Usage:       "Pipelines commands.",
			Subcommands: pipelines.GetCommands(),
			Category:    commandNamespacesCategory,
		},
		{
			Name:        cliutils.CmdCompletion,
			Usage:       "Generate autocomplete scripts.",
			Subcommands: completion.GetCommands(),
			Category:    otherCategory,
		},
		{
			Name:        cliutils.CmdPlugin,
			Usage:       "Plugins handling commands.",
			Subcommands: plugins.GetCommands(),
			Category:    commandNamespacesCategory,
		},
		{
			Name:        cliutils.CmdConfig,
			Aliases:     []string{"c"},
			Usage:       "Server configurations commands.",
			Subcommands: config.GetCommands(),
			Category:    commandNamespacesCategory,
		},
		{
			Name:        cliutils.CmdProject,
			Hidden:      true,
			Usage:       "Project commands.",
			Subcommands: project.GetCommands(),
			Category:    otherCategory,
		},
		{
			Name:         "ci-setup",
			Hidden:       true,
			Usage:        cisetup.GetDescription(),
			HelpName:     corecommon.CreateUsage("ci-setup", cisetup.GetDescription(), cisetup.Usage),
			ArgsUsage:    common.CreateEnvVars(),
			BashComplete: corecommon.CreateBashCompletionFunc(),
			Category:     otherCategory,
			Action: func(c *cli.Context) error {
				return cisetupcommand.RunCiSetupCmd()
			},
		},
		{
			Name:   "setup",
			Hidden: true,
			Flags:  cliutils.GetCommandFlags(cliutils.Setup),
			Action: SetupCmd,
		},
		{
			Name:   "intro",
			Hidden: true,
			Flags:  cliutils.GetCommandFlags(cliutils.Intro),
			Action: IntroCmd,
		},
		{
			Name:     cliutils.CmdOptions,
			Usage:    "Show all supported environment variables.",
			Category: otherCategory,
			Action: func(*cli.Context) {
				fmt.Println(common.GetGlobalEnvVars())
			},
		},
		{
			Name:         "login",
			Usage:        loginDocs.GetDescription(),
			HelpName:     corecommon.CreateUsage("login", loginDocs.GetDescription(), loginDocs.Usage),
			BashComplete: corecommon.CreateBashCompletionFunc(),
			Category:     otherCategory,
			Action:       login.LoginCmd,
		},
		{
			Hidden:       true,
			Name:         "how",
			Usage:        aiDocs.GetDescription(),
			HelpName:     corecommon.CreateUsage("how", aiDocs.GetDescription(), aiDocs.Usage),
			BashComplete: corecommon.CreateBashCompletionFunc(),
			Category:     otherCategory,
			Action:       ai.HowCmd,
		},
		{
			Name:         "access-token-create",
			Aliases:      []string{"atc"},
			Flags:        cliutils.GetCommandFlags(cliutils.AccessTokenCreate),
			Usage:        tokenDocs.GetDescription(),
			HelpName:     corecommon.CreateUsage("atc", tokenDocs.GetDescription(), tokenDocs.Usage),
			UsageText:    tokenDocs.GetArguments(),
			ArgsUsage:    common.CreateEnvVars(),
			BashComplete: corecommon.CreateBashCompletionFunc(),
			Category:     otherCategory,
			Action:       token.AccessTokenCreateCmd,
		},
	}

	securityCmds, err := ConvertEmbeddedPlugin(securityCLI.GetJfrogCliSecurityApp())
	if err != nil {
		return nil, err
	}
	allCommands := append(slices.Clone(cliNameSpaces), securityCmds...)
	allCommands = append(allCommands, utils.GetPlugins()...)
	allCommands = append(allCommands, buildtools.GetCommands()...)
	allCommands = append(allCommands, lifecycle.GetCommands()...)
	return append(allCommands, buildtools.GetBuildToolsHelpCommands()...), nil
}

// Embedded plugins are CLI plugins that are embedded in the JFrog CLI and not require any installation.
// This function converts an embedded plugin to a cli.Command slice to be registered as commands of the cli.
func ConvertEmbeddedPlugin(jfrogPlugin components.App) (converted []cli.Command, err error) {
	for i := range jfrogPlugin.Subcommands {
		// commands name-space without category are considered as 'other' category
		if jfrogPlugin.Subcommands[i].Category == "" {
			jfrogPlugin.Subcommands[i].Category = otherCategory
		}
	}
	if converted, err = components.ConvertAppCommands(jfrogPlugin); err != nil {
		err = fmt.Errorf("failed adding '%s' embedded plugin commands. Last error: %s", jfrogPlugin.Name, err.Error())
	}
	return
}

func getAppHelpTemplate() string {
	return `NAME:
   ` + coreutils.GetCliExecutableName() + ` - {{.Usage}}

USAGE:
   {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}} {{if .VisibleFlags}}[global options]{{end}}{{if .Commands}} command [command options]{{end}} [arguments...]{{end}}
   {{if .Version}}
VERSION:
   {{.Version}}
   {{end}}{{if len .Authors}}
AUTHOR(S):
   {{range .Authors}}{{ . }}{{end}}
   {{end}}{{if .VisibleCommands}}
COMMANDS:{{range .VisibleCategories}}{{if .Name}}

   {{.Name}}:{{end}}{{range .VisibleCommands}}
     {{join .Names ", "}}{{ "\t" }}{{if .Description}}{{.Description}}{{else}}{{.Usage}}{{end}}{{end}}{{end}}{{end}}{{if .VisibleFlags}}

GLOBAL OPTIONS:
   {{range .VisibleFlags}}{{.}}
   {{end}}
{{end}}
`
}

func SetupCmd(c *cli.Context) error {
	format := setupcore.Human
	formatFlag := c.String("format")
	if formatFlag == string(setupcore.Machine) {
		format = setupcore.Machine
	}
	return envsetup.RunEnvSetupCmd(c, format)
}

func IntroCmd(_ *cli.Context) error {
	ci, err := clientutils.GetBoolEnvValue(coreutils.CI, false)
	if ci || err != nil {
		return err
	}
	clientlog.Output()
	clientlog.Output(coreutils.PrintTitle(fmt.Sprintf("Thank you for installing version %s of JFrog CLI! 🐸", cliutils.CliVersion)))
	var serverExists bool
	serverExists, err = coreconfig.IsServerConfExists()
	if serverExists || err != nil {
		return err
	}
	clientlog.Output(coreutils.PrintTitle("So what's next?"))
	clientlog.Output()
	clientlog.Output(coreutils.PrintTitle("Authenticate with your JFrog Platform by running one of the following two commands:"))
	clientlog.Output()
	clientlog.Output("jf login")
	clientlog.Output(coreutils.PrintTitle("or"))
	clientlog.Output("jf c add")
	return nil
}
