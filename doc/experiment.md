# Experiments

Experiments are defined in `internal/experiment.go`.

To enable experiments for local development, create a file containing
experiments in YAML format.

Set environment variable `GO_DISCOVERY_CONFIG_DYNAMIC` to that filename.

Example:

```
experiments:
  - name: sidenav
    rollout: 100
```

You can also run `devtools/cmd/create_experiment_config/main.go` to generate an
experiment.yaml file in the root of the repository. This YAML file will contain all
experiments defined in internal/experiment.go at the time of execution.

You can then set `GO_DISCOVERY_CONFIG_DYNAMIC` that filename.
