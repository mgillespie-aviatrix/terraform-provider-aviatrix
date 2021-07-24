package goaviatrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type Version struct {
	CID           string `form:"CID,omitempty"`
	Action        string `form:"action,omitempty"`
	TargetVersion string `form:"version,omitempty"`
	Version       string `json:"version,omitempty"`
}

type UpgradeResp struct {
	Return  bool   `json:"return"`
	Results string `json:"results"`
	Reason  string `json:"reason"`
}

type VersionInfoResults struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
}

type VersionInfoResp struct {
	Return  bool               `json:"return"`
	Results VersionInfoResults `json:"results"`
	Reason  string             `json:"reason"`
}

type AviatrixVersion struct {
	Major int64
	Minor int64
	Build int64
}

type VersionInfo struct {
	Current  *AviatrixVersion
	Previous *AviatrixVersion
}

func (av *AviatrixVersion) String(includeBuild bool) string {
	if includeBuild {
		return fmt.Sprintf("%d.%d.%d", av.Major, av.Minor, av.Build)
	}
	return fmt.Sprintf("%d.%d", av.Major, av.Minor)

}

func (c *Client) Upgrade(version *Version, upgradeGateways bool) error {
	if version.Version == "" {
		return errors.New("no target version is set")
	}
	Url, err := url.Parse(c.baseURL)
	if err != nil {
		return errors.New(("url Parsing failed for upgrade") + err.Error())
	}
	var action string
	upgradeController := url.Values{}
	upgradeController.Add("CID", c.CID)
	if upgradeGateways {
		action = "upgrade"
		upgradeController.Add("action", action)
		if version.Version != "latest" {
			upgradeController.Add("version", version.Version)
		}
	} else {
		action = "upgrade_platform"
		upgradeController.Add("action", action)
		upgradeController.Add("gateway_list", "")
		upgradeController.Add("software_version", version.Version)
	}

	for i := 0; ; i++ {
		Url.RawQuery = upgradeController.Encode()
		resp, err := c.Get(Url.String(), nil)
		if err != nil {
			return fmt.Errorf("HTTP Get %s failed: %v", action, err)
		}
		var data UpgradeResp
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		bodyString := buf.String()
		bodyIoCopy := strings.NewReader(bodyString)
		if err = json.NewDecoder(bodyIoCopy).Decode(&data); err != nil {
			return fmt.Errorf("Json Decode %s failed: %v\n Body: %s", action, err, bodyString)
		}
		if !data.Return {
			if strings.Contains(data.Reason, "Active upgrade in progress.") && i < 3 {
				log.Infof("Active upgrade is in progress. Retry after 60 secs...")
				time.Sleep(60 * time.Second)
				continue
			}
			return fmt.Errorf("rest API %s Get failed: %v", action, data.Reason)
		}
		c.Login()
		break
	}
	return nil
}

func (c *Client) UpgradeGateway(gateway *Gateway) error {
	form := map[string]string{
		"action":           "upgrade_selected_gateway",
		"CID":              c.CID,
		"gateway_list":     gateway.GwName,
		"software_version": gateway.SoftwareVersion,
		"image_version":    gateway.ImageVersion,
	}
	return c.PostAPI(form["action"], form, BasicCheck)
}

func (c *Client) GetCurrentVersion() (string, *AviatrixVersion, error) {
	Url, err := url.Parse(c.baseURL)
	if err != nil {
		return "", nil, errors.New(("url Parsing failed for list_version_info") + err.Error())
	}
	listVersionInfo := url.Values{}
	listVersionInfo.Add("CID", c.CID)
	listVersionInfo.Add("action", "list_version_info")
	Url.RawQuery = listVersionInfo.Encode()
	resp, err := c.Get(Url.String(), nil)

	if err != nil {
		return "", nil, errors.New("HTTP Get list_version_info failed: " + err.Error())
	}
	var data VersionInfoResp
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	bodyString := buf.String()
	bodyIoCopy := strings.NewReader(bodyString)
	if err = json.NewDecoder(bodyIoCopy).Decode(&data); err != nil {
		return "", nil, errors.New("Json Decode list_version_info failed: " + err.Error() + "\n Body: " + bodyString)
	}
	if !data.Return {
		return "", nil, errors.New("Rest API list_version_info Get failed: " + data.Reason)
	}

	curVersion, aVer, err := ParseVersion(data.Results.CurrentVersion)
	if err != nil {
		return "", aVer, err
	}

	return curVersion, aVer, nil
}

func (c *Client) GetVersionInfo() (*VersionInfo, error) {
	form := map[string]string{
		"action": "list_version_info",
		"CID":    c.CID,
	}
	var data struct {
		Results struct {
			PreviousVersion string `json:"previous_version"`
			CurrentVersion  string `json:"current_version"`
		}
	}
	err := c.GetAPI(&data, form["action"], form, BasicCheck)
	if err != nil {
		return nil, err
	}
	_, current, err := ParseVersion(data.Results.CurrentVersion)
	if err != nil {
		return nil, fmt.Errorf("could not parse current version: %v", err)
	}
	_, previous, err := ParseVersion(data.Results.PreviousVersion)
	if err != nil {
		return nil, fmt.Errorf("could not parse previous version: %v", err)
	}
	return &VersionInfo{
		Current:  current,
		Previous: previous,
	}, nil
}

func (c *Client) Pre32Upgrade() error {
	privateBaseURL := strings.Replace(c.baseURL, "/v1/api", "/v1/backend1", 1)
	params := &Version{
		Action: "userconnect_release",
		CID:    c.CID,
	}
	path := privateBaseURL
	for i := 0; ; i++ {
		resp, err := c.Post(path, params)
		if err != nil {
			return errors.New("HTTP Post userconnect_release failed: " + err.Error())
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			body, _ := ioutil.ReadAll(resp.Body)
			log.Tracef("response %s", body)
			if strings.Contains(string(body), "in progress") && i < 3 {
				log.Infof("Active upgrade is in progress. Retry after 60 secs...")
				time.Sleep(60 * time.Second)
			} else {
				break
			}
		} else {
			return fmt.Errorf("status code %d", resp.StatusCode)
		}
	}
	return nil
}

func (c *Client) GetLatestVersion() (string, error) {
	Url, err := url.Parse(c.baseURL)
	if err != nil {
		return "", errors.New(("url Parsing failed for list_version_info") + err.Error())
	}
	listVersionInfo := url.Values{}
	listVersionInfo.Add("CID", c.CID)
	listVersionInfo.Add("action", "list_version_info")
	listVersionInfo.Add("latest_version", strconv.FormatBool(true))
	Url.RawQuery = listVersionInfo.Encode()
	resp, err := c.Get(Url.String(), nil)
	if err != nil {
		return "", errors.New("HTTP Get list_version_info failed: " + err.Error())
	}
	var data VersionInfoResp
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	bodyString := buf.String()
	bodyIoCopy := strings.NewReader(bodyString)
	if err = json.NewDecoder(bodyIoCopy).Decode(&data); err != nil {
		return "", errors.New("Json Decode list_version_info failed: " + err.Error() + "\n Body: " + bodyString)
	}
	if !data.Return {
		return "", errors.New("Rest API list_version_info Get failed: " + data.Reason)
	}

	latestVersion, _, err := ParseVersion(data.Results.LatestVersion)
	if err != nil {
		return "", err
	}
	return latestVersion, nil
}

func ParseVersion(version string) (string, *AviatrixVersion, error) {
	version = strings.TrimPrefix(version, "UserConnect-")
	if version == "" {
		return "", nil, errors.New("unable to parse version information since it is empty")
	}

	parts := strings.Split(version, ".")
	aver := &AviatrixVersion{}
	var err1, err2, err3 error
	aver.Major, err1 = strconv.ParseInt(parts[0], 10, 0)
	if len(parts) >= 2 {
		if strings.Contains(parts[1], "-") {
			aver.Minor, err2 = strconv.ParseInt(strings.Split(parts[1], "-")[0], 10, 0)
		} else {
			aver.Minor, err2 = strconv.ParseInt(parts[1], 10, 0)
		}
	} else {
		return "", aver, errors.New("unable to get latest version when parsing version information")
	}
	if len(parts) >= 3 {
		aver.Build, err3 = strconv.ParseInt(parts[2], 10, 0)
	}
	if err1 != nil || err2 != nil || err3 != nil {
		return "", aver, errors.New("unable to get latest version when parsing version information")
	}
	return strconv.FormatInt(aver.Major, 10) + "." + strconv.FormatInt(aver.Minor, 10), aver, nil
}

func (c *Client) GetCompatibleImageVersion(ctx context.Context, cloudType int, softwareVersion string) (string, error) {
	form := map[string]string{
		"action":          "get_compatible_image_version",
		"CID":             c.CID,
		"software_verion": softwareVersion,
		"cloud_type":      strconv.Itoa(cloudType),
	}
	var data struct {
		Results struct {
			ImageVersion string `json:"image_version"`
		}
	}
	err := c.GetAPIContext(ctx, &data, form["action"], form, BasicCheck)
	if err != nil {
		return "", err
	}
	return data.Results.ImageVersion, nil
}
