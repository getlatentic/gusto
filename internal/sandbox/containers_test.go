package sandbox

// plain is the containers of a machine where nothing is set.
func plain() Containers {
	return Containers{Runtime: "docker", Image: "gusto:dev", CPUs: "1", Memory: "3g", ToolCPUs: "0.5", ToolMemory: "512m"}
}

// with is plain with another runtime and image.
func with(runtime, image string) Containers {
	c := plain()
	c.Runtime, c.Image = runtime, image
	return c
}
