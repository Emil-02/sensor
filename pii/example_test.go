package pii_test

import (
	"fmt"

	"github.com/Emil-02/sensor/pii"
)

func ExampleRedact() {
	out, findings, _ := pii.Redact("NIK 3201014508900001, WA 0812-3456-7890", pii.Options{})
	fmt.Println(out)
	fmt.Println(len(findings), "data pribadi")
	// Output:
	// NIK ************0001, WA ****-****-7890
	// 2 data pribadi
}

func ExampleScanner_Redact() {
	s := pii.New(pii.Phone, pii.Email)
	out, _, _ := s.Redact("NIK 3201014508900001, email budi@contoh.id", pii.Options{Mode: pii.ModeLabel})
	fmt.Println(out)
	// Output: NIK 3201014508900001, email [EMAIL]
}
