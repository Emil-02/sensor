//go:build js && wasm

package main

import "syscall/js"

func main() {
	str := func(args []js.Value, i int) string {
		if i < len(args) && args[i].Type() == js.TypeString {
			return args[i].String()
		}
		return ""
	}
	js.Global().Set("sensor", js.ValueOf(map[string]any{
		"scan": js.FuncOf(func(_ js.Value, args []js.Value) any {
			return scanJSON(str(args, 0), str(args, 1))
		}),
		"redact": js.FuncOf(func(_ js.Value, args []js.Value) any {
			return redactJSON(str(args, 0), str(args, 1), str(args, 2))
		}),
		"ready": true,
	}))
	select {}
}
