package cliutils

import (
	"fmt"
	"os"
	"strings"

	speccore "github.com/jfrog/jfrog-cli-core/v2/common/spec"

	"github.com/codegangsta/cli"
	coreCommonCommands "github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-cli/utils/summary"
	"github.com/jfrog/jfrog-client-go/utils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/pkg/errors"
)

type CommanDomain string

const (
	Rt CommanDomain = "rt"
	Ds CommanDomain = "ds"
	Xr CommanDomain = "xr"
)

// Error modes (how should the application behave when the CheckError function is invoked):
type OnError string

func init() {
	// Initialize cli-core values.
	cliUserAgent := os.Getenv(UserAgent)
	if cliUserAgent != "" {
		cliUserAgentName, cliUserAgentVersion := splitAgentNameAndVersion(cliUserAgent)
		coreutils.SetCliUserAgentName(cliUserAgentName)
		coreutils.SetCliUserAgentVersion(cliUserAgentVersion)
	} else {
		coreutils.SetCliUserAgentName(ClientAgent)
		coreutils.SetCliUserAgentVersion(CliVersion)
	}
	coreutils.SetClientAgentName(ClientAgent)
	coreutils.SetClientAgentVersion(CliVersion)
}

// Splits the full agent name to its name and version.
// The full agent name needs to be the agent name and version separated by a slash ('/').
// If the full agent name doesn't include a version, then it's returned as the agent name and an empty string is returned as the agent version.
func splitAgentNameAndVersion(fullAgentName string) (string, string) {
	var agentName, agentVersion string
	lastSlashIndex := strings.LastIndex(fullAgentName, "/")
	if lastSlashIndex == -1 {
		agentName = fullAgentName
	} else {
		agentName = fullAgentName[:lastSlashIndex]
		agentVersion = fullAgentName[lastSlashIndex+1:]
	}

	return agentName, agentVersion
}

func GetCliError(err error, success, failed int, failNoOp bool) error {
	switch coreutils.GetExitCode(err, success, failed, failNoOp) {
	case coreutils.ExitCodeError:
		{
			var errorMessage string
			if err != nil {
				errorMessage = err.Error()
			}
			return coreutils.CliError{ExitCode: coreutils.ExitCodeError, ErrorMsg: errorMessage}
		}
	case coreutils.ExitCodeFailNoOp:
		return coreutils.CliError{ExitCode: coreutils.ExitCodeFailNoOp, ErrorMsg: "No errors, but also no files affected (fail-no-op flag)."}
	default:
		return nil
	}
}

type DetailedSummaryRecord struct {
	Source string `json:"source,omitempty"`
	Target string `json:"target"`
}

type ExtendedDetailedSummaryRecord struct {
	DetailedSummaryRecord
	Sha256 string `json:"sha256"`
}

// Print summary report.
// a given non-nil error will pass through and be returned as is if no other errors are raised.
// In case of a nil error, the current function error will be returned.
func summaryPrintError(summaryError, originalError error) error {
	if originalError != nil {
		if summaryError != nil {
			log.Error(summaryError)
		}
		return originalError
	}
	return summaryError
}

func PrintSummaryReport(success, failed int, originalErr error) error {
	basicSummary, mErr := CreateSummaryReportString(success, failed, originalErr)
	if mErr != nil {
		return summaryPrintError(mErr, originalErr)
	}
	log.Output(basicSummary)
	return summaryPrintError(mErr, originalErr)
}

// Prints a summary report.
// If a resultReader is provided, we will iterate over the result and print a detailed summary including the affected files.
func PrintDetailedSummaryReport(success, failed int, reader *content.ContentReader, printExtendedDetails bool, originalErr error) error {
	basicSummary, mErr := CreateSummaryReportString(success, failed, originalErr)
	if mErr != nil {
		return summaryPrintError(mErr, originalErr)
	}
	// A reader wasn't provided, prints the basic summary json and return.
	if reader == nil {
		log.Output(basicSummary)
		return summaryPrintError(mErr, originalErr)
	}
	reader.Reset()
	defer reader.Close()
	writer, mErr := content.NewContentWriter("files", false, true)
	if mErr != nil {
		log.Output(basicSummary)
		return summaryPrintError(mErr, originalErr)
	}
	// We remove the closing curly bracket in order to append the affected files array using a responseWriter to write directly to stdout.
	basicSummary = strings.TrimSuffix(basicSummary, "\n}") + ","
	log.Output(basicSummary)
	defer log.Output("}")
	readerLength, _ := reader.Length()
	// If the reader is empty we will print an empty array.
	if readerLength == 0 {
		log.Output("  files: []")
	} else {
		for transferDetails := new(clientutils.FileTransferDetails); reader.NextRecord(transferDetails) == nil; transferDetails = new(clientutils.FileTransferDetails) {
			writer.Write(getDetailedSummaryRecord(transferDetails, printExtendedDetails))
		}
	}
	mErr = writer.Close()
	return summaryPrintError(mErr, originalErr)
}

// Get the detailed summary record.
// In case of an upload/publish commands we want to print sha256 of the uploaded file in addition to the source and the target.
func getDetailedSummaryRecord(transferDetails *clientutils.FileTransferDetails, extendDetailedSummary bool) interface{} {
	record := DetailedSummaryRecord{
		Source: transferDetails.SourcePath,
		Target: transferDetails.TargetPath,
	}
	if extendDetailedSummary {
		extendedRecord := ExtendedDetailedSummaryRecord{
			DetailedSummaryRecord: record,
			Sha256:                transferDetails.Sha256,
		}
		return extendedRecord
	}
	return record
}

func PrintBuildInfoSummaryReport(succeeded bool, sha256 string, originalErr error) error {
	success, failed := 1, 0
	if !succeeded {
		success, failed = 0, 1
	}
	summary, mErr := CreateBuildInfoSummaryReportString(success, failed, sha256, originalErr)
	if mErr != nil {
		return summaryPrintError(mErr, originalErr)
	}
	log.Output(summary)
	return summaryPrintError(mErr, originalErr)
}

func CreateSummaryReportString(success, failed int, err error) (string, error) {
	summaryReport := summary.GetSummaryReport(success, failed, err)
	content, mErr := summaryReport.Marshal()
	if errorutils.CheckError(mErr) != nil {
		return "", mErr
	}
	return utils.IndentJson(content), mErr
}

func CreateBuildInfoSummaryReportString(success, failed int, sha256 string, err error) (string, error) {
	buildInfoSummary := summary.NewBuildInfoSummary(success, failed, sha256, err)
	content, mErr := buildInfoSummary.Marshal()
	if errorutils.CheckError(mErr) != nil {
		return "", mErr
	}
	return utils.IndentJson(content), mErr
}

func PrintHelpAndReturnError(msg string, context *cli.Context) error {
	log.Error(msg + " " + GetDocumentationMessage())
	cli.ShowCommandHelp(context, context.Command.Name)
	return errors.New(msg)
}

// This function indicates whether the command should be executed without
// confirmation warning or not.
// If the --quiet option was sent, it is used to determine whether to prompt the confirmation or not.
// If not, the command will prompt the confirmation, unless the CI environment variable was set to true.
func GetQuietValue(c *cli.Context) bool {
	if c.IsSet("quiet") {
		return c.Bool("quiet")
	}

	return getCiValue()
}

// This function indicates whether the command should be executed in
// an interactive mode.
// If the --interactive option was sent, it is used to determine the mode.
// If not, the mode will be interactive, unless the CI environment variable was set to true.
func GetInteractiveValue(c *cli.Context) bool {
	if c.IsSet("interactive") {
		return c.BoolT("interactive")
	}

	return !getCiValue()
}

// Return true if the CI environment variable was set to true.
func getCiValue() bool {
	var ci bool
	var err error
	if ci, err = clientutils.GetBoolEnvValue(coreutils.CI, false); err != nil {
		return false
	}
	return ci
}

func GetVersion() string {
	return CliVersion
}

func GetDocumentationMessage() string {
	return "You can read the documentation at https://www.jfrog.com/confluence/display/CLI/JFrog+CLI"
}

func GetBuildName(buildName string) string {
	return getOrDefaultEnv(buildName, coreutils.BuildName)
}

func GetBuildUrl(buildUrl string) string {
	return getOrDefaultEnv(buildUrl, BuildUrl)
}

func GetEnvExclude(envExclude string) string {
	return getOrDefaultEnv(envExclude, EnvExclude)
}

// Return argument if not empty or retrieve from environment variable
func getOrDefaultEnv(arg, envKey string) string {
	if arg != "" {
		return arg
	}
	return os.Getenv(envKey)
}

func ShouldOfferConfig() (bool, error) {
	exists, err := config.IsServerConfExists()
	if err != nil || exists {
		return false, err
	}

	var ci bool
	if ci, err = clientutils.GetBoolEnvValue(coreutils.CI, false); err != nil {
		return false, err
	}
	if ci {
		config.SaveServersConf(make([]*config.ServerDetails, 0))
		return false, nil
	}

	msg := fmt.Sprintf("To avoid this message in the future, set the %s environment variable to true.\n"+
		"The CLI commands require the URL and authentication details\n"+
		"Configuring JFrog CLI with these parameters now will save you having to include them as command options.\n"+
		"You can also configure these parameters later using the 'jfrog c' command.\n"+
		"Configure now?", coreutils.CI)
	confirmed := coreutils.AskYesNo(msg, false)
	if !confirmed {
		config.SaveServersConf(make([]*config.ServerDetails, 0))
		return false, nil
	}
	return true, nil
}

func CreateServerDetailsFromFlags(c *cli.Context) (details *config.ServerDetails) {
	details = new(config.ServerDetails)
	details.Url = clientutils.AddTrailingSlashIfNeeded(c.String(url))
	details.ArtifactoryUrl = clientutils.AddTrailingSlashIfNeeded(c.String(configRtUrl))
	details.DistributionUrl = clientutils.AddTrailingSlashIfNeeded(c.String(configDistUrl))
	details.XrayUrl = clientutils.AddTrailingSlashIfNeeded(c.String(configXrUrl))
	details.MissionControlUrl = clientutils.AddTrailingSlashIfNeeded(c.String(configMcUrl))
	details.PipelinesUrl = clientutils.AddTrailingSlashIfNeeded(c.String(configPlUrl))
	details.User = c.String(user)
	details.Password = c.String(password)
	details.SshKeyPath = c.String(sshKeyPath)
	details.SshPassphrase = c.String(sshPassphrase)
	details.AccessToken = c.String(accessToken)
	details.ClientCertPath = c.String(clientCertPath)
	details.ClientCertKeyPath = c.String(clientCertKeyPath)
	details.ServerId = c.String(serverId)
	details.InsecureTls = c.Bool(InsecureTls)
	return
}

func IsLegacyGoPublish(c *cli.Context) bool {
	return c.Command.Name == "go-publish" && c.NArg() > 1
}

func GetSpec(c *cli.Context, isDownload bool) (specFiles *speccore.SpecFiles, err error) {
	specFiles, err = speccore.CreateSpecFromFile(c.String("spec"), coreutils.SpecVarsStringToMap(c.String("spec-vars")))
	if err != nil {
		return nil, err
	}
	// Override spec with CLI options
	for i := 0; i < len(specFiles.Files); i++ {
		if isDownload {
			specFiles.Get(i).Pattern = strings.TrimPrefix(specFiles.Get(i).Pattern, "/")
		}
		OverrideFieldsIfSet(specFiles.Get(i), c)
	}
	return
}

// If `fieldName` exist in the cli args, read it to `field` as a string.
func overrideStringIfSet(field *string, c *cli.Context, fieldName string) {
	if c.IsSet(fieldName) {
		*field = c.String(fieldName)
	}
}

// If `fieldName` exist in the cli args, read it to `field` as an array split by `;`.
func overrideArrayIfSet(field *[]string, c *cli.Context, fieldName string) {
	if c.IsSet(fieldName) {
		*field = nil
		for _, singleValue := range strings.Split(c.String(fieldName), ";") {
			*field = append(*field, singleValue)
		}
	}
}

// If `fieldName` exist in the cli args, read it to `field` as a int.
func overrideIntIfSet(field *int, c *cli.Context, fieldName string) {
	if c.IsSet(fieldName) {
		*field = c.Int(fieldName)
	}
}

func offerConfig(c *cli.Context, domain CommanDomain) (*coreConfig.ServerDetails, error) {
	confirmed, err := ShouldOfferConfig()
	if !confirmed || err != nil {
		return nil, err
	}
	details := createServerDetailsFromFlags(c, domain)
	configCmd := coreCommonCommands.NewConfigCommand().SetDefaultDetails(details).SetInteractive(true).SetEncPassword(true)
	err = configCmd.Config()
	if err != nil {
		return nil, err
	}

	return configCmd.ServerDetails()
}

// Exclude refreshable tokens paramater should be true when working with external tools (build tools, curl, etc)
// or when sending requests not via ArtifactoryHttpClient.
func CreateServerDetailsWithConfigOffer(c *cli.Context, excludeRefreshableTokens bool, domain CommanDomain) (*coreConfig.ServerDetails, error) {
	createdDetails, err := offerConfig(c, domain)
	if err != nil {
		return nil, err
	}
	if createdDetails != nil {
		return createdDetails, err
	}

	details := createServerDetailsFromFlags(c, domain)
	// If urls or credentials were passed as options, use options as they are.
	// For security reasons, we'd like to avoid using part of the connection details from command options and the rest from the config.
	// Either use command options only or config only.
	if credentialsChanged(details) {
		return details, nil
	}

	// Else, use details from config for requested serverId, or for default server if empty.
	confDetails, err := coreCommonCommands.GetConfig(details.ServerId, excludeRefreshableTokens)
	if err != nil {
		return nil, err
	}

	// Take InsecureTls value from options since it is not saved in config.
	confDetails.InsecureTls = details.InsecureTls
	confDetails.Url = clientutils.AddTrailingSlashIfNeeded(confDetails.Url)
	confDetails.DistributionUrl = clientutils.AddTrailingSlashIfNeeded(confDetails.DistributionUrl)

	// Create initial access token if needed.
	if !excludeRefreshableTokens {
		err = coreConfig.CreateInitialRefreshableTokensIfNeeded(confDetails)
		if err != nil {
			return nil, err
		}
	}

	return confDetails, nil
}

func createServerDetailsFromFlags(c *cli.Context, domain CommanDomain) (details *coreConfig.ServerDetails) {
	details = CreateServerDetailsFromFlags(c)
	switch domain {
	case Rt:
		details.ArtifactoryUrl = details.Url
	case Xr:
		details.XrayUrl = details.Url
	case Ds:
		details.DistributionUrl = details.Url
	}
	details.Url = ""

	return
}

func credentialsChanged(details *coreConfig.ServerDetails) bool {
	return details.Url != "" || details.ArtifactoryUrl != "" || details.DistributionUrl != "" || details.XrayUrl != "" ||
		details.User != "" || details.Password != "" || details.SshKeyPath != "" || details.SshPassphrase != "" || details.AccessToken != "" ||
		details.ClientCertKeyPath != "" || details.ClientCertPath != ""
}

// This function checks whether the command received --help as a single option.
// If it did, the command's help is shown and true is returned.
// This function should be used iff the SkipFlagParsing option is used.
func ShowCmdHelpIfNeeded(c *cli.Context) (bool, error) {
	if len(c.Args()) != 1 {
		return false, nil
	}
	if c.Args()[0] == "--help" || c.Args()[0] == "-h" {
		err := cli.ShowCommandHelp(c, c.Command.Name)
		return true, err
	}
	return false, nil
}

func GetFileSystemSpec(c *cli.Context) (fsSpec *speccore.SpecFiles, err error) {
	fsSpec, err = speccore.CreateSpecFromFile(c.String("spec"), coreutils.SpecVarsStringToMap(c.String("spec-vars")))
	if err != nil {
		return
	}
	// Override spec with CLI options
	for i := 0; i < len(fsSpec.Files); i++ {
		fsSpec.Get(i).Target = strings.TrimPrefix(fsSpec.Get(i).Target, "/")
		OverrideFieldsIfSet(fsSpec.Get(i), c)
	}
	return
}

func OverrideFieldsIfSet(spec *speccore.File, c *cli.Context) {
	overrideArrayIfSet(&spec.Exclusions, c, "exclusions")
	overrideArrayIfSet(&spec.SortBy, c, "sort-by")
	overrideIntIfSet(&spec.Offset, c, "offset")
	overrideIntIfSet(&spec.Limit, c, "limit")
	overrideStringIfSet(&spec.SortOrder, c, "sort-order")
	overrideStringIfSet(&spec.Props, c, "props")
	overrideStringIfSet(&spec.TargetProps, c, "target-props")
	overrideStringIfSet(&spec.ExcludeProps, c, "exclude-props")
	overrideStringIfSet(&spec.Build, c, "build")
	overrideStringIfSet(&spec.Project, c, "project")
	overrideStringIfSet(&spec.ExcludeArtifacts, c, "exclude-artifacts")
	overrideStringIfSet(&spec.IncludeDeps, c, "include-deps")
	overrideStringIfSet(&spec.Bundle, c, "bundle")
	overrideStringIfSet(&spec.Recursive, c, "recursive")
	overrideStringIfSet(&spec.Flat, c, "flat")
	overrideStringIfSet(&spec.Explode, c, "explode")
	overrideStringIfSet(&spec.Regexp, c, "regexp")
	overrideStringIfSet(&spec.IncludeDirs, c, "include-dirs")
	overrideStringIfSet(&spec.ValidateSymlinks, c, "validate-symlinks")
	overrideStringIfSet(&spec.Symlinks, c, "symlinks")
	overrideStringIfSet(&spec.Transitive, c, "transitive")
	overrideStringIfSet(&spec.PublicGpgKey, c, "gpg-key")
}

func FixWinPathsForFileSystemSourcedCmds(uploadSpec *speccore.SpecFiles, c *cli.Context) {
	if coreutils.IsWindows() {
		for i, file := range uploadSpec.Files {
			uploadSpec.Files[i].Pattern = fixWinPathBySource(file.Pattern, c.IsSet("spec"))
			for j, exclusion := range uploadSpec.Files[i].Exclusions {
				// If exclusions are set, they override the spec value
				uploadSpec.Files[i].Exclusions[j] = fixWinPathBySource(exclusion, c.IsSet("spec") && !c.IsSet("exclusions"))
			}
		}
	}
}

func fixWinPathsForDownloadCmd(uploadSpec *speccore.SpecFiles, c *cli.Context) {
	if coreutils.IsWindows() {
		for i, file := range uploadSpec.Files {
			uploadSpec.Files[i].Target = fixWinPathBySource(file.Target, c.IsSet("spec"))
		}
	}
}

func fixWinPathBySource(path string, fromSpec bool) string {
	if strings.Count(path, "/") > 0 {
		// Assuming forward slashes - not doubling backslash to allow regexp escaping
		return ioutils.UnixToWinPathSeparator(path)
	}
	if fromSpec {
		// Doubling backslash only for paths from spec files (that aren't forward slashed)
		return ioutils.DoubleWinPathSeparator(path)
	}
	return path
}
