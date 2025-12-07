package main

import (
	"encoding/json"
	"fmt"
)

// cmdHooksPrint prints the merged hook configuration (global + local).
func cmdHooksPrint() error {
	if loadedHooks == nil {
		cfg, err := loadHooks()
		if err != nil {
			return err
		}
		loadedHooks = cfg
	}
	if loadedHooks == nil {
		fmt.Println("No hooks configured")
		return nil
	}
	out, err := json.MarshalIndent(loadedHooks, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}
