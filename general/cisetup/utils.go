package cisetup

import (
	artUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/general/cisetup"
	utilsconfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/config"
	"github.com/jfrog/jfrog-client-go/xray"
)

const (
	// Repo types
	Remote  = "remote"
	Virtual = "virtual"
	Local   = "local"

	NewRepository = "[Create new repository]"

	// Repos defaults
	MavenLocalDefaultName    = "maven-central-local"
	MavenRemoteDefaultName   = "maven-central-remote"
	MavenRemoteDefaultUrl    = "https://repo.maven.apache.org/maven2"
	MavenVirtualDefaultName  = "maven-virtual"
	GradleLocalDefaultName   = "gradle-local"
	GradleRemoteDefaultName  = "gradle-remote"
	GradleRemoteDefaultUrl   = "https://repo.maven.apache.org/maven2"
	GradleVirtualDefaultName = "gradle-virtual"
	NpmLocalDefaultName      = "npm-local"
	NpmRemoteDefaultName     = "npm-remote"
	NpmRemoteDefaultUrl      = "https://registry.npmjs.org"
	NpmVirtualDefaultName    = "npm-virtual"

	// Build commands defaults
	mavenDefaultBuildCmd  = "mvn clean install"
	gradleDefaultBuildCmd = "gradle clean artifactoryPublish"
	npmDefaultBuildCmd    = "npm install"
)

var RepoDefaultName = map[cisetup.Technology]map[string]string{
	cisetup.Maven: {
		Local:   MavenLocalDefaultName,
		Remote:  MavenRemoteDefaultName,
		Virtual: MavenVirtualDefaultName,
	},
	cisetup.Gradle: {
		Local:   GradleLocalDefaultName,
		Remote:  GradleRemoteDefaultName,
		Virtual: GradleVirtualDefaultName,
	},
	cisetup.Npm: {
		Local:   NpmLocalDefaultName,
		Remote:  NpmRemoteDefaultName,
		Virtual: NpmVirtualDefaultName,
	},
}

var RepoRemoteDefaultUrl = map[cisetup.Technology]string{
	cisetup.Maven:  MavenRemoteDefaultUrl,
	cisetup.Gradle: GradleRemoteDefaultUrl,
	cisetup.Npm:    NpmRemoteDefaultUrl,
}

var buildCmdByTech = map[cisetup.Technology]string{
	cisetup.Maven:  mavenDefaultBuildCmd,
	cisetup.Gradle: gradleDefaultBuildCmd,
	cisetup.Npm:    npmDefaultBuildCmd,
}

func CreateXrayServiceManager(serviceDetails *utilsconfig.ServerDetails) (*xray.XrayServicesManager, error) {
	xrayDetails, err := serviceDetails.CreateXrayAuthConfig()
	serviceConfig, err := config.NewConfigBuilder().
		SetServiceDetails(xrayDetails).
		Build()
	if err != nil {
		return nil, err
	}
	return xray.New(serviceConfig)
}

func GetAllRepos(serviceDetails *utilsconfig.ServerDetails, repoType, packageType string) (*[]services.RepositoryDetails, error) {
	servicesManager, err := artUtils.CreateServiceManager(serviceDetails, -1, false)
	if err != nil {
		return nil, err
	}
	filterParams := services.RepositoriesFilterParams{RepoType: repoType, PackageType: packageType}
	return servicesManager.GetAllRepositoriesFiltered(filterParams)
}

func GetVirtualRepo(serviceDetails *utilsconfig.ServerDetails, repoKey string) (*services.VirtualRepositoryBaseParams, error) {
	servicesManager, err := artUtils.CreateServiceManager(serviceDetails, -1, false)
	if err != nil {
		return nil, err
	}
	virtualRepoDetails := services.VirtualRepositoryBaseParams{}
	err = servicesManager.GetRepository(repoKey, &virtualRepoDetails)
	return &virtualRepoDetails, err
}

func contains(arr []string, str string) bool {
	for _, element := range arr {
		if element == str {
			return true
		}
	}
	return false
}

func CreateLocalRepo(serviceDetails *utilsconfig.ServerDetails, technologyType cisetup.Technology, repoName string) error {
	servicesManager, err := artUtils.CreateServiceManager(serviceDetails, -1, false)
	if err != nil {
		return err
	}
	params := services.NewLocalRepositoryBaseParams()
	params.PackageType = string(technologyType)
	params.Key = repoName
	return servicesManager.CreateLocalRepositoryWithParams(params)
}

func CreateRemoteRepo(serviceDetails *utilsconfig.ServerDetails, technologyType cisetup.Technology, repoName, remoteUrl string) error {
	servicesManager, err := artUtils.CreateServiceManager(serviceDetails, -1, false)
	if err != nil {
		return err
	}
	params := services.NewRemoteRepositoryBaseParams()
	params.PackageType = string(technologyType)
	params.Key = repoName
	params.Url = remoteUrl
	return servicesManager.CreateRemoteRepositoryWithParams(params)
}

func CreateVirtualRepo(serviceDetails *utilsconfig.ServerDetails, technologyType cisetup.Technology, repoName string, repositories ...string) error {
	servicesManager, err := artUtils.CreateServiceManager(serviceDetails, -1, false)
	if err != nil {
		return err
	}
	params := services.NewVirtualRepositoryBaseParams()
	params.PackageType = string(technologyType)
	params.Key = repoName
	params.Repositories = repositories
	return servicesManager.CreateVirtualRepositoryWithParams(params)
}
