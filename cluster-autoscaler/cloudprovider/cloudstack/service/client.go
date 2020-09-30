package service

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// APIClient is an interface to communicate to CloudStack via HTTP calls
type APIClient interface {
	// NewRequest makes an API request to configured management server
	NewRequest(api string, args map[string]string, out interface{}) (map[string]interface{}, error)
}

// Config contains the parameters used to configure a new APIClient
type Config struct {
	APIKey    string
	SecretKey string
	Endpoint  string
}

// client implements the APIClient interface
type client struct {
	config *Config
	client *http.Client
}

// func (client *client) encodeRequestParams(params url.Values) string {

// 	params.

// 	var buf bytes.Buffer

// 	for key, value := range params {
// 		if buf.Len() > 0 {
// 			buf.WriteByte('&')
// 		}
// 		buf.WriteString(key)
// 		buf.WriteString("=")
// 		buf.WriteString(url.QueryEscape(value))
// 	}

// 	if params == nil {
// 		return ""
// 	}

// 	keys := make([]string, 0, len(params))
// 	for key := range params {
// 		keys = append(keys, key)
// 	}
// 	sort.Strings(keys)

// 	var buf bytes.Buffer
// 	for _, key := range keys {
// 		value := params.Get(key)
// 		if buf.Len() > 0 {
// 			buf.WriteByte('&')
// 		}
// 		buf.WriteString(key)
// 		buf.WriteString("=")
// 		buf.WriteString(url.QueryEscape(value))
// 	}
// 	return buf.String()
// }

func (client *client) getResponseData(data map[string]interface{}) map[string]interface{} {
	for k := range data {
		if strings.HasSuffix(k, "response") {
			return data[k].(map[string]interface{})
		}
	}
	return nil
}

func (client *client) pollAsyncJob(jobID string, out interface{}) (map[string]interface{}, error) {
	timeout := time.NewTimer(1 * time.Hour)
	ticker := time.NewTicker(10 * time.Second)

	defer ticker.Stop()
	defer timeout.Stop()

	for {
		select {
		case <-timeout.C:
			return nil, fmt.Errorf("Timed out getting result for jobid : %s", jobID)

		case <-ticker.C:
			result, err := client.newRequest("queryAsyncJobResult", map[string]string{
				"jobid": jobID,
			}, false, out)
			if err != nil {
				return result, err
			}

			status := result["jobstatus"].(float64)
			switch status {
			case 0:
				continue
			case 1:
				data, err := json.Marshal(result["jobresult"])
				json.Unmarshal(data, out)
				return result["jobresult"].(map[string]interface{}), err
			case 2:
				fmt.Printf("async API failed : %v\n", result)
				return result, errors.New("async API failed for job " + jobID)
			default:
				return result, errors.New("async API failed for job " + jobID)
			}
		}
	}
}

// NewRequest makes an API request to configured management server
func (client *client) NewRequest(api string, args map[string]string, out interface{}) (map[string]interface{}, error) {
	return client.newRequest(api, args, true, out)
}

func (client *client) createQueryString(api string, args map[string]string) url.Values {
	params := make(url.Values)
	for key, value := range args {
		params.Add(key, value)
	}

	params.Add("command", api)
	params.Add("response", "json")

	params.Add("apiKey", client.config.APIKey)
	encodedParams := params.Encode()

	mac := hmac.New(sha1.New, []byte(client.config.SecretKey))
	mac.Write([]byte(strings.Replace(strings.ToLower(encodedParams), "+", "%20", -1)))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	encodedParams = fmt.Sprintf("%s&signature=%s", encodedParams, url.QueryEscape(signature))

	return params
}

func (client *client) newRequest(api string, args map[string]string, async bool, out interface{}) (map[string]interface{}, error) {
	params := client.createQueryString(api, args)

	requestURL := fmt.Sprintf("%s?%s", client.config.Endpoint, params)
	fmt.Println("NewAPIRequest API request URL:", requestURL)

	response, err := client.client.Get(requestURL)
	if err != nil {
		return nil, err
	}
	fmt.Println("NewAPIRequest response status code:", response.StatusCode)

	body, _ := ioutil.ReadAll(response.Body)
	// fmt.Println("NewAPIRequest response body:", string(body))

	var data map[string]interface{}
	_ = json.Unmarshal([]byte(body), &data)

	if data != nil && async {
		if jobResponse := client.getResponseData(data); jobResponse != nil && jobResponse["jobid"] != nil {
			jobID := jobResponse["jobid"].(string)
			return client.pollAsyncJob(jobID, out)
		}
	}

	if apiResponse := client.getResponseData(data); apiResponse != nil {
		if _, ok := apiResponse["errorcode"]; ok {
			return nil, fmt.Errorf("(HTTP %v, error code %v) %v", apiResponse["errorcode"], apiResponse["cserrorcode"], apiResponse["errortext"])
		}
		if out != nil {
			json.Unmarshal([]byte(body), out)
		}
		return apiResponse, nil
	}

	return nil, errors.New("failed to decode response")
}

func newHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	client.Timeout = time.Duration(time.Duration(3600) * time.Second)
	return client
}

// NewAPIClient returns a new APIClient
func NewAPIClient(config *Config) APIClient {
	httpClient := newHTTPClient()
	return &client{
		config: config,
		client: httpClient,
	}
}
