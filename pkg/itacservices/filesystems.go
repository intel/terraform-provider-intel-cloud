package itacservices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"terraform-provider-intelcloud/pkg/itacservices/common"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	retry "github.com/sethvargo/go-retry"
)

var (
	getAllFilesystemsURL         = "{{.Host}}/v1/cloudaccounts/{{.Cloudaccount}}/filesystems?metadata.filterType=ComputeGeneral"
	createFilesystemsURL         = "{{.Host}}/v1/cloudaccounts/{{.Cloudaccount}}/filesystems"
	getFilesystemByResourceId    = "{{.Host}}/v1/cloudaccounts/{{.Cloudaccount}}/filesystems/id/{{.ResourceId}}"
	deleteFilesystemByResourceId = "{{.Host}}/v1/cloudaccounts/{{.Cloudaccount}}/filesystems/id/{{.ResourceId}}"
	getLoginCredentials          = "{{.Host}}/v1/cloudaccounts/{{.Cloudaccount}}/filesystems/id/{{.ResourceId}}/user"
)

type Filesystems struct {
	FilesystemList []Filesystem `json:"items"`
}

type Filesystem struct {
	Metadata struct {
		ResourceId   string `json:"resourceId"`
		Cloudaccount string `json:"cloudAccountId"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		CreatedAt    string `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		Request struct {
			Size string `json:"storage"`
		} `json:"request"`
		StorageClass     string `json:"storageClass"`
		AccessMode       string `json:"accessModes"`
		FilesystemType   string `json:"filesystemType"`
		Encrypted        bool   `json:"Encrypted"`
		AvailabilityZone string `json:"availabilityZone"`
	} `json:"spec"`
	Status struct {
		Phase string `json:"phase"`
		Mount struct {
			ClusterAddr    string `json:"clusterAddr"`
			ClusterVersion string `json:"clusterVersion"`
			Namespace      string `json:"namespace"`
			UserName       string `json:"username"`
			Password       string `json:"password"`
			FilesystemName string `json:"filesystemName"`
		} `json:"mount"`
	} `json:"status"`
}

type LoginCreds struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type FilesystemCreateRequest struct {
	Metadata struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"metadata"`
	Spec struct {
		Request struct {
			Size string `json:"storage"`
		} `json:"request"`
		StorageClass     string `json:"storageClass"`
		AccessMode       string `json:"accessModes"`
		FilesystemType   string `json:"filesystemType"`
		InstanceType     string `json:"instanceType"`
		Encrypted        bool   `json:"Encrypted"`
		AvailabilityZone string `json:"availabilityZone"`
	} `json:"spec"`
}

func (client *IDCServicesClient) GetFilesystems(ctx context.Context) (*Filesystems, error) {
	params := struct {
		Host         string
		Cloudaccount string
	}{
		Host:         *client.Host,
		Cloudaccount: *client.Cloudaccount,
	}

	// Parse the template string with the provided data
	parsedURL, err := common.ParseString(getAllFilesystemsURL, params)
	if err != nil {
		return nil, fmt.Errorf("error parsing the url")
	}

	retcode, retval, err := common.MakeGetAPICall(ctx, parsedURL, *client.Apitoken, nil)
	tflog.Debug(ctx, "filesystem read api", map[string]any{"retcode": retcode, "retval": string(retval)})
	if err != nil {
		return nil, fmt.Errorf("error reading filesystems")
	}

	if retcode != http.StatusOK {
		return nil, common.MapHttpError(retcode)
	}
	filesystems := Filesystems{}
	if err := json.Unmarshal(retval, &filesystems); err != nil {
		return nil, fmt.Errorf("error parsing filesystem response")
	}

	var password *string
	if len(filesystems.FilesystemList) != 0 {
		// generate credentials. Single pair of credentials is used for all
		// filesystems
		// get login credentials
		password, err = client.GenerateFilesystemLoginCredentials(ctx, filesystems.FilesystemList[0].Metadata.ResourceId)
		if err != nil {
			return nil, fmt.Errorf("error generating filesystem login credentials")
		}
	}

	for idx, _ := range filesystems.FilesystemList {
		filesystems.FilesystemList[idx].Status.Mount.Password = *password
	}

	return &filesystems, nil
}

func (client *IDCServicesClient) GenerateFilesystemLoginCredentials(ctx context.Context, resourceId string) (*string, error) {
	getLoginParams := struct {
		Host         string
		Cloudaccount string
		ResourceId   string
	}{
		Host:         *client.Host,
		Cloudaccount: *client.Cloudaccount,
		ResourceId:   resourceId,
	}
	// Parse the template string with the provided data
	parsedURL, err := common.ParseString(getLoginCredentials, getLoginParams)
	if err != nil {
		return nil, fmt.Errorf("error parsing the url")
	}

	retcode, retval, err := common.MakeGetAPICall(ctx, parsedURL, *client.Apitoken, nil)
	if err != nil {
		return nil, fmt.Errorf("error generating login credentials")
	}
	if retcode != http.StatusOK {
		return nil, common.MapHttpError(retcode)
	}
	creds := LoginCreds{}
	if err := json.Unmarshal(retval, &creds); err != nil {
		return nil, fmt.Errorf("error parsing filesystem credentials response")
	}
	return &creds.Password, nil
}

func (client *IDCServicesClient) CreateFilesystem(ctx context.Context, in *FilesystemCreateRequest) (*Filesystem, error) {
	params := struct {
		Host         string
		Cloudaccount string
	}{
		Host:         *client.Host,
		Cloudaccount: *client.Cloudaccount,
	}

	// Parse the template string with the provided data
	parsedURL, err := common.ParseString(createFilesystemsURL, params)
	if err != nil {
		return nil, fmt.Errorf("error parsing the url")
	}

	inArgs, err := json.MarshalIndent(in, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("error parsing input arguments")
	}

	tflog.Debug(ctx, "filesystem create api", map[string]any{"url": parsedURL, "inArgs": string(inArgs)})
	retcode, retval, err := common.MakePOSTAPICall(ctx, parsedURL, *client.Apitoken, inArgs)
	tflog.Debug(ctx, "filesystem create api", map[string]any{"retcode": retcode, "retval": string(retval)})
	if err != nil {
		return nil, fmt.Errorf("error reading filesystem create response")
	}
	if retcode != http.StatusOK {
		return nil, common.MapHttpError(retcode)
	}

	filesystem := &Filesystem{}
	if err := json.Unmarshal(retval, filesystem); err != nil {
		return nil, fmt.Errorf("error parsing filesystem response")
	}

	backoffTimer := retry.NewConstant(5 * time.Second)
	backoffTimer = retry.WithMaxDuration(300*time.Second, backoffTimer)

	if err := retry.Do(ctx, backoffTimer, func(_ context.Context) error {
		filesystem, err = client.GetFilesystemByResourceId(ctx, filesystem.Metadata.ResourceId)
		if err != nil {
			return fmt.Errorf("error reading filesystem state")
		}
		if filesystem.Status.Phase == "FSReady" {
			return nil
		} else if filesystem.Status.Phase == "FSFailed" {
			return fmt.Errorf("filesystem state failed")
		} else {
			return retry.RetryableError(fmt.Errorf("filesystem state not ready, retry again"))
		}
	}); err != nil {
		return nil, fmt.Errorf("filesystem state not ready after maximum retries")
	}

	password, err := client.GenerateFilesystemLoginCredentials(ctx, filesystem.Metadata.ResourceId)
	if err != nil {
		return nil, fmt.Errorf("error generating login credentials")
	}
	filesystem.Status.Mount.Password = *password

	return filesystem, nil
}

func (client *IDCServicesClient) GetFilesystemByResourceId(ctx context.Context, resourceId string) (*Filesystem, error) {
	params := struct {
		Host         string
		Cloudaccount string
		ResourceId   string
	}{
		Host:         *client.Host,
		Cloudaccount: *client.Cloudaccount,
		ResourceId:   resourceId,
	}

	// Parse the template string with the provided data
	parsedURL, err := common.ParseString(getFilesystemByResourceId, params)
	if err != nil {
		return nil, fmt.Errorf("error parsing the url")
	}

	retcode, retval, err := common.MakeGetAPICall(ctx, parsedURL, *client.Apitoken, nil)
	if err != nil {
		return nil, fmt.Errorf("error reading filesystem by resource id")
	}

	if retcode != http.StatusOK {
		return nil, common.MapHttpError(retcode)
	}

	tflog.Debug(ctx, "filesystem read api", map[string]any{"retcode": retcode})
	filesystem := Filesystem{}
	if err := json.Unmarshal(retval, &filesystem); err != nil {
		return nil, fmt.Errorf("error parsing filesystem response")
	}
	return &filesystem, nil
}

func (client *IDCServicesClient) DeleteFilesystemByResourceId(ctx context.Context, resourceId string) error {
	params := struct {
		Host         string
		Cloudaccount string
		ResourceId   string
	}{
		Host:         *client.Host,
		Cloudaccount: *client.Cloudaccount,
		ResourceId:   resourceId,
	}

	// Parse the template string with the provided data
	parsedURL, err := common.ParseString(deleteFilesystemByResourceId, params)
	if err != nil {
		return fmt.Errorf("error parsing the url")
	}

	retcode, retval, err := common.MakeDeleteAPICall(ctx, parsedURL, *client.Apitoken, nil)
	if err != nil {
		return fmt.Errorf("error deleting filesystem by resource id")
	}

	tflog.Debug(ctx, "filesystem delete api", map[string]any{"retcode": retcode, "retval": string(retval)})

	if retcode != http.StatusOK {
		return common.MapHttpError(retcode)
	}

	tflog.Debug(ctx, "filesystem delete api", map[string]any{"retcode": retcode})

	return nil
}