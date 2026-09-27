package framebench

import "encoding/json"

func canonicalScenario(scenario Scenario) ([]byte, error) { return json.Marshal(scenario) }
