package cloudstack

import (
	"bytes"
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
	"sort"
	"strings"
	"time"
)

type cloudStackClient interface {
	NewAPIRequest(api string, args map[string]string, async bool, out interface{}) (map[string]interface{}, error)
}

type client struct {
	config *acsConfig
	client *http.Client
}

func (client *client) encodeRequestParams(params url.Values) string {
	if params == nil {
		return ""
	}

	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	for _, key := range keys {
		value := params.Get(key)
		if buf.Len() > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(key)
		buf.WriteString("=")
		buf.WriteString(url.QueryEscape(value))
	}
	return buf.String()
}

func (client *client) getResponseData(data map[string]interface{}) map[string]interface{} {
	for k := range data {
		if strings.HasSuffix(k, "response") {
			return data[k].(map[string]interface{})
		}
	}
	return nil
}

func (client *client) pollAsyncJob(jobID string, out interface{}) (map[string]interface{}, error) {
	var timeout float64 = 3600
	for timeout > 0.0 {
		startTime := time.Now()
		queryResult, queryError := client.NewAPIRequest("queryAsyncJobResult", map[string]string{
			"jobid": jobID,
		}, false, out)
		diff := time.Duration(1*time.Second).Nanoseconds() - time.Now().Sub(startTime).Nanoseconds()
		if diff > 0 {
			time.Sleep(time.Duration(diff) * time.Nanosecond)
		}
		timeout = timeout - time.Now().Sub(startTime).Seconds()
		if queryError != nil {
			return queryResult, queryError
		}
		jobStatus := queryResult["jobstatus"].(float64)
		fmt.Println("JOB STATUS : ", jobStatus)
		if jobStatus == 0 {
			continue
		}
		if jobStatus == 1 {
			b, err := json.Marshal(queryResult["jobresult"])
			json.Unmarshal(b, out)
			return queryResult["jobresult"].(map[string]interface{}), err

		}
		if jobStatus == 2 {
			return queryResult, errors.New("async API failed for job " + jobID)
		}
	}
	return nil, errors.New("async API job query timed out")
}

// NewAPIRequest makes an API request to configured management server
func (client *client) NewAPIRequest(api string, args map[string]string, async bool, out interface{}) (map[string]interface{}, error) {
	params := make(url.Values)
	params.Add("command", api)
	for key, value := range args {
		params.Add(key, value)
	}
	params.Add("response", "json")

	var encodedParams string
	var err error

	params.Add("apiKey", client.config.APIKey)
	encodedParams = client.encodeRequestParams(params)

	mac := hmac.New(sha1.New, []byte(client.config.SecretKey))
	mac.Write([]byte(strings.Replace(strings.ToLower(encodedParams), "+", "%20", -1)))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	encodedParams = encodedParams + fmt.Sprintf("&signature=%s", url.QueryEscape(signature))

	requestURL := fmt.Sprintf("%s?%s", client.config.Endpoint, encodedParams)
	fmt.Println("NewAPIRequest API request URL:", requestURL)

	response, err := client.client.Get(requestURL)
	if err != nil {
		return nil, err
	}
	fmt.Println("NewAPIRequest response status code:", response.StatusCode)

	body, _ := ioutil.ReadAll(response.Body)
	fmt.Println("NewAPIRequest response body:", string(body))

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

func newClient(config *acsConfig) cloudStackClient {
	httpClient := newHTTPClient()
	return &client{
		config: config,
		client: httpClient,
	}
}
