package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/client"
)

func main() {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	// e.g. "nginx:alpine"
	dist, err := c.DistributionInspect(ctx, "nginx:alpine", "")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Digest: %s\n", dist.Descriptor.Digest)
	
	// Check a local image digest
	img, _, err := c.ImageInspectWithRaw(ctx, "nginx:alpine")
	if err != nil {
		fmt.Println("Error inspecting local image:", err)
		return
	}
	fmt.Printf("Local RepoDigests: %v\n", img.RepoDigests)
	fmt.Printf("Local Id: %s\n", img.ID)
}
