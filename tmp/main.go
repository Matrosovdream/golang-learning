package main

import "fmt"

func main() {

	dec := 255
	hex := 0xFF
	oct := 0o377
	bin := 0b1111_1111

	fmt.Println(dec, hex, oct, bin)
	fmt.Println(dec == hex && hex == oct && oct == bin)

}
