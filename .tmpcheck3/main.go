package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	data, err := os.ReadFile("/faber/result/answers.json")
	if err != nil {
		panic(err)
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		panic(err)
	}
	fmt.Println("valid json")
}
