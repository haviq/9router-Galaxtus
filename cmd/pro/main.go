package main

import (
	"fmt"
	"net/http"

	"9router/proxy/internal/pro"
)

func main() {
	g := pro.NewGateway(
		pro.GetEnv("UPSTREAM", "https://api.provider-tujuan.com/v1"),
		pro.GetEnv("PROVIDER_KEY", "sk-xxx"),
	)

	http.Handle("/v1/chat/completions", g)
	http.Handle("/chat/completions", g)

	port := pro.GetEnv("PORT", "1337")
	fmt.Printf("9router-pro pipeline jalan di :%s — empat proses aktif\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

