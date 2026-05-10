package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
)

func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	
	vols, err := cli.VolumeList(context.Background(), volume.ListOptions{})
	fmt.Printf("VolumeList count: %d\n", len(vols.Volumes))
	if len(vols.Volumes) > 0 {
		b, _ := json.MarshalIndent(vols.Volumes[0], "", "  ")
		fmt.Println("VolumeList First:", string(b))
	}

	du, err := cli.DiskUsage(context.Background(), types.DiskUsageOptions{})
	if err != nil {
		panic(err)
	}
	
	fmt.Printf("DiskUsage Volumes count: %d\n", len(du.Volumes))
	if len(du.Volumes) > 0 {
		b, _ := json.MarshalIndent(du.Volumes[0], "", "  ")
		fmt.Println("DiskUsage First:", string(b))
	}
}
