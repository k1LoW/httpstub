module github.com/k1LoW/httpstub

go 1.24.7

require (
	github.com/IGLOU-EU/go-wildcard/v2 v2.1.0
	github.com/golang/mock v1.6.0
	github.com/pb33f/libopenapi v0.28.1
	github.com/pb33f/libopenapi-validator v0.8.1
)

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/pb33f/jsonpath v0.1.2 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.2 // indirect
	golang.org/x/text v0.30.0 // indirect
)

// Licensing error. ref: https://github.com/k1LoW/httpstub/issues/118
retract [v0.9.0, v0.23.0]
