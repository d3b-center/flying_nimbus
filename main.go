package main

import "flying_nimbus/cmd/services"

func main() {
	app := services.NewApp()
	if err := app.Run(); err != nil {
		panic(err)
	}
}
