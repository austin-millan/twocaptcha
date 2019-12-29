package twocaptcha

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/valyala/fasthttp"
)

// Constants, shouldn't be modified (left as var because slices un-constable)
var validTypes = []string{"recaptchaV2", "recaptchaV3", "funcaptcha"}
var validV3Scores = []string{".1", ".3", ".9"}
var capRequestURL = "https://2captcha.com/in.php?json=1"
var capResultURL = "https://2captcha.com/res.php?json=1"
var captchaErrors = map[string]error{
	// Automatically handled errors
	"CAPCHA_NOT_READY":         errors.New("handled by program")
	"ERROR_NO_SLOT_AVAILABLE":    errors.New("handled by program"),
	// API key errors (for both endpoints)
	"ERROR_WRONG_USER_KEY":     errors.New("invalidly formatted api key"),
	"ERROR_KEY_DOES_NOT_EXIST": errors.New("invalid api key"),
	// https://2captcha.com/in.php
	"ERROR_ZERO_BALANCE":         errors.New("[in] empty account balance"),
	"IP_BANNED":                  errors.New("[in] ip banned, contact 2captcha"),
	"ERROR_BAD_TOKEN_OR_PAGEURL": errors.New("[in] recapv2 invalid token/pageurl"),
	"ERROR_GOOGLEKEY":            errors.New("[in] recapv2 invalid sitekey"),
	"MAX_USER_TURN":              errors.New("[in] too many requests, temp 10s ban"),
	// https://2captcha.com/res.php
	"CAPTCHA_NOT_READY":        errors.New("[res] captcha not ready"),
	"ERROR_CAPTCHA_UNSOLVABLE": errors.New("[res] unsolvable captcha"),
	"ERROR_WRONG_ID_FORMAT":    errors.New("[res] invalidly formatted captcha id"),
	"ERROR_WRONG_CAPTCHA_ID":   errors.New("[res] invalid captcha id"),
	"ERROR_BAD_DUPLICATES":     errors.New("[res] not enough matches"),
	"ERROR_EMPTY_ACTION":       errors.New("[res] action not found"),
}

// CaptchaInstance represents an individual captcha instance interfacing with the 2captcha API.
// Different combinations of captcha type and parameters (captchaInfo) require separate instances;
// for instance, even for the same website solving both RecaptchaV2 and RecaptchaV3 require two
// separate instances.
type CaptchaInstance struct {
	APIKey        string
	CaptchaType   string // must be within validTypes
	CreateTaskURL string
	// recaptchaV2 - sitekey, siteurl
	// recaptchaV3 - sitkeey, siteurl, action, minScore
	// funcaptcha  - sitekey, surl, siteurl
	SettingInfo map[string]string
	// "timeBetweenReqs" int: time between checking requests
	HTTPClient *fasthttp.Client
}

type captchaResponse struct {
	Status   int    // 0 usually represents error, 1 represents valid request
	Response string `json:"request"` // response body (called request)
}

// checkResponse checks whether a request was successful - for instance, some websites send
// zero-length responses and 503s. This function primarily acts like a just-in-case and
// currently does nothing.
func checkResponse(rawResponse *fasthttp.Response) (result bool) {
	result = true
	return result
}

func checkError(responseStruct *captchaResponse) (errKey string, err error) {
	if responseStruct.Status == 0 {
		for key, value := range captchaErrors {
			if responseStruct.Response == key {
				errKey = key
				err = value // error
				break
			}
		}
	}
	return errKey, err
}

// keyInMap checks whether a given key exists within a map ([string]string)
func keyInMap(inputMap map[string]string, key string) (result bool) {
	_, result = inputMap[key]
	return result
}

// stringInSlice checks whether an input slice (of strings) contains a string
func stringInSlice(inputSlice []string, key string) (result bool) {
	for _, item := range inputSlice {
		if key == item {
			result = true
			break
		}
	}
	return result
}

// NewInstance creates and populates a new CaptchaInstance. If any error is encountered during
// initialization, NewInstance returns an empty CaptchaInstance and whatever error was found, else
// it returns the populated instance and nil error.
func NewInstance(
	apiKey string, captchaType string, captchaParams map[string]string, settingParams map[string]string,
) (instance CaptchaInstance, finalErr error) {
OuterLoop:
	for {
		// Verify that initialization key(s) (timeBetweenReqs) exist within map (settingParams).
		if !(keyInMap(settingParams, "timeBetweenReqs")) {
			finalErr = errors.New("missing parameter(s) within settingParams")
			break OuterLoop
		}

		// Verify that passed captchaType within valid types (validTypes) for proper initialization.
		if !stringInSlice(validTypes, captchaType) {
			finalErr = errors.New("invalid captcha type")
			break OuterLoop
		}

		// Verify that captcha-specific keys exist within map (captchaParams), then pass entire
		// captchaParams map into instance after switch statement completes.
		switch captchaType {
		case "recaptchaV2":
			if !(keyInMap(captchaParams, "sitekey") && keyInMap(captchaParams, "siteurl")) {
				finalErr = errors.New("missing parameter(s) within captchaParams for recaptchaV2")
				break OuterLoop
			}
		case "recaptchaV3":
			if !(keyInMap(captchaParams, "sitekey") && keyInMap(captchaParams, "siteurl") &&
				keyInMap(captchaParams, "action") && keyInMap(captchaParams, "minscore")) {
				finalErr = errors.New("missing parameter(s) within captchaParams for recaptchaV3")
				break OuterLoop
			}
			// Verify inputted score within allowed inputs
			if !stringInSlice(validV3Scores, captchaParams["minscore"]) {
				finalErr = errors.New("invalid recaptchaV3 score (.1/.3/.9)")
			}
		case "funcaptcha":
			if !(keyInMap(captchaParams, "key") && keyInMap(captchaParams, "surl") &&
				keyInMap(captchaParams, "siteurl")) {
				finalErr = errors.New("missing parameter(s) within captchaParams for funcaptcha")
				break OuterLoop
			}
		default: // shouldn't happen because captchaType previously verified
			finalErr = errors.New("invalid captcha type (this shouldn't happen)")
			break OuterLoop
		}

		httpClient := &fasthttp.Client{}

		var balanceStruct captchaResponse
		requestURL := capResultURL + "&action=getBalance&key=" + apiKey
		// Verify api key by checking remaining balance - don't do anything if balance empty
		for retryRequest := true; retryRequest; {
			request := fasthttp.AcquireRequest()
			request.Header.SetMethod("GET")
			request.SetRequestURI(requestURL)
			response := fasthttp.AcquireResponse()
			httpClient.Do(request, response)
			if checkResponse(response) {
				if err := json.Unmarshal(response.Body(), &balanceStruct); err != nil {
					finalErr = errors.New("error unmarshalling (this shouldn't happen)")
					fasthttp.ReleaseRequest(request)
					fasthttp.ReleaseResponse(response)
					break OuterLoop
				}
				retryRequest = false
			}
			fasthttp.ReleaseRequest(request)
			fasthttp.ReleaseResponse(response)
		}

		if _, err := checkError(&balanceStruct); err != nil {
			finalErr = err
			break OuterLoop
		}

		createTaskURL := capRequestURL + "&key=" + instance.APIKey
		switch instance.CaptchaType {
		case "recaptchaV2":
			requestURL += "method=userrecaptcha&googlekey=" + captchaParams["sitekey"] +
				"&pageurl=" + captchaParams["siteurl"]
		case "recaptchaV3":
			requestURL += "method=userrecaptcha&version=v3&googlekey=" + captchaParams["sitekey"] +
				"&pageurl=" + captchaParams["siteurl"] + "&action=" + captchaParams["action"] +
				"&min_score=" + captchaParams["minScore"]
		case "funcaptcha":
			requestURL += "method=funcaptcha&publickey=" + captchaParams["sitekey"] +
				"&surl=" + captchaParams["surl"] + "&pageurl=" + captchaParams["siteurl"]

		default:
			finalErr = errors.New("invalid captcha type (this shouldn't happen!)")
			break OuterLoop
		}

		instance.APIKey = apiKey
		instance.CaptchaType = captchaType
		instance.CreateTaskURL = createTaskURL
		instance.SettingInfo = settingParams
		instance.HTTPClient = httpClient
		break OuterLoop
	}

	return instance, finalErr
}

// SolveCaptcha solves for a given captcha type and returns the solution and error, if any.
// If any errors are encountered, SolveCaptcha returns an empty solution string and error.
func (instance *CaptchaInstance) SolveCaptcha() (solution string, finalErr error) {
OuterLoop:
	for {
		var checkSolutionURL string
		// Doing Atoi alot takes ... resources?
		// - Maybe turn SettingInfo into interface{} vs string map
		// - Remove SettingInfo and instead have each setting as a field
		secondsToSleep, _ := strconv.Atoi(instance.SettingInfo["timeBetweenReqs"])
		timeToSleep := time.Second * time.Duration(secondsToSleep) 

	CreateTaskLoop:
		for {
			// Create captcha solving task using instance's CreateTaskURL
			for retryRequest := true; retryRequest; {
				request := fasthttp.AcquireRequest()
				request.Header.SetMethod("GET")
				request.SetRequestURI(instance.CreateTaskURL)
				response := fasthttp.AcquireResponse()
				instance.HTTPClient.Do(request, response)
				if checkResponse(response) {
					if err := json.Unmarshal(response.Body(), &taskStruct); err != nil {
						finalErr = errors.New("error unmarshalling (this shouldn't happen)")
						fasthttp.ReleaseRequest(request)
						fasthttp.ReleaseResponse(response)
						break OuterLoop
					}
					retryRequest = false
				}
				fasthttp.ReleaseRequest(request)
				fasthttp.ReleaseResponse(response)
			}

			if errKey, err := checkError(&taskStruct); err != nil {
				if errKey == "ERROR_NO_SLOT_AVAILABLE" {
					time.Sleep(timeToSleep)
					continue
				}
				finalErr = err
				break OuterLoop
			}

			captchaTaskID := taskStruct.Response // Should only include task id
			checkSolutionURL = capResultURL + "&key=" + instance.APIKey + "&action=get&id=" + captchaTaskID
			break CreateTaskLoop
		}

	SolutionLoop:
		for {
			var solutionStruct captchaResponse
			// Check for captcha completion, else sleep and retry
			for retryRequest := true; retryRequest; {
				request := fasthttp.AcquireRequest()
				request.Header.SetMethod("GET")
				request.SetRequestURI(checkSolutionURL)
				response := fasthttp.AcquireResponse()
				instance.HTTPClient.Do(request, response)
				if checkResponse(response) {
					if err := json.Unmarshal(response.Body(), &solutionStruct); err != nil {
						finalErr = errors.New("error unmarshalling (this shouldn't happen)")
						fasthttp.ReleaseRequest(request)
						fasthttp.ReleaseResponse(response)
						break OuterLoop
					}
					retryRequest = false
				}
				fasthttp.ReleaseRequest(request)
				fasthttp.ReleaseResponse(response)
			}

			if _, err := checkError(&taskStruct); err != nil {
				if errKey == "CAPCHA_NOT_READY" {
					time.Sleep(timeToSleep)
					continue
				}
				finalErr = err
				break OuterLoop
			}

			solution = solutionStruct.Response
			break SolutionLoop
		}
	}

	return solution, finalErr
}
