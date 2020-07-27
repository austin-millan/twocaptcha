# twocaptcha

Golang package for solving captchas through the 2captcha API

## Install

```go
go get -u "https://github.com/austin-millan/twocaptcha"
```

## Usage

```go
func main() {
  apiKey := "insert_apikey_here"
  settingParams := twocaptcha.SettingInfo{TimeBetweenRequests: 10}

  instance, err := twocaptcha.NewInstance(apiKey, settingParams)
  if err != nil {
    // do something with err
  }

  solution, err := instance.SolveRecaptchaV2("insert_sitekey_here", "insert_siteurl_here")
  if err != nil {
    // do something with err
  }
  fmt.Printf("%s", solution)
}
```
