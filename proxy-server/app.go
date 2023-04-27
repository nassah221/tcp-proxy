package main

type App struct {
	Name    string
	Ports   map[int]struct{}
	Targets []string
	Current int
}

type Apps []App

func NewApps(cfg *Config) Apps {
	var apps Apps

	for _, a := range cfg.Apps {
		app := App{
			Name:    a.Name,
			Targets: a.Targets,
			Ports:   make(map[int]struct{}),
		}

		for _, p := range a.Ports {
			app.Ports[p] = struct{}{}
		}

		apps = append(apps, app)
	}

	return apps
}
