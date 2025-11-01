package banner

import (
	"fmt"
)

// prints the version message
const version = "v0.0.2"

func PrintVersion() {
	fmt.Printf("Current emailextractor version %s\n", version)
}

// Prints the Colorful banner
func PrintBanner() {
	banner := `
                           _  __             __                      __              
  ___   ____ ___   ____ _ (_)/ /___   _  __ / /_ _____ ____ _ _____ / /_ ____   _____
 / _ \ / __  __ \ / __  // // // _ \ | |/_// __// ___// __  // ___// __// __ \ / ___/
/  __// / / / / // /_/ // // //  __/_>  < / /_ / /   / /_/ // /__ / /_ / /_/ // /    
\___//_/ /_/ /_/ \__,_//_//_/ \___//_/|_| \__//_/    \__,_/ \___/ \__/ \____//_/
`
    fmt.Printf("%s\n%75s\n\n", banner, "Current emailextractor version "+version)
}
