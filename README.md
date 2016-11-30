# pincloud

Cloud SDK users often want to pin the SDK to old versions to preserve some functionality or usage. Doing so makes it difficult for them to adopt new features, or even just features from different SDK versions.

pincloud is a tiny `gcloud` wrapper that uses a heuristic to figure out which command is being run, and then consults a config file to decide which "real" version of `gcloud` to use.

## Default gcloud and SDK

The first `gcloud` in `$PATH` that is not this binary is referred to as the "default `gcloud`". It's the `gcloud` that would have been run if pincloud had not been installed. The SDK that the default `gcloud` comes from is called the default SDK.

## The configuration

A file `~/.config/pincloud/pins.cfg` will store the pincloud configuration. By example, it will look something like the following.

```
gcloud alpha container builds: /path/to/sdk-135.0.0/gcloud
gcloud app deploy: /path/to/sdk/102.0.0/gcloud preview
gcloud compute: 135.0.0
```

The first entry pins the CloudBuild CLI to version 135, which the user installed somewhere on the system.

The second entry pins `gcloud app deploy` to the preview version of that command in version 102.

The third entry pins `gcloud compute` commands to the SDK in `~/.config/pincloud/versions/135.0.0`. If the pin is not an absolute path, it is assumed to be a short version found in the directory where pincloud lee[s] managed SDK versions.

The rule is: if pincloud recognizes the command or command group on the left, it replaces arg 0 with the args on the right. So, things like `preview` or `beta` can be inserted.

If the command is not recognized as one of the pins, the default `gcloud` is used.

## The heuristic

To guess which command or command group is being run, pincloud will ignore all arguments that begin with a dash. As a consequence, long flags that take values must use the `--flag=value` variant. Short flags that take values will not be usable; their longer equivalents must be used instead.

The remaining positional arguments will be prefix-matched against the pin configuration keys, in the order they appear in the pin config, to decide which "real" `gcloud` is to be used.

## Managing pins

pincloud also has a facility to help manage pins. To add a new SDK version, it copies the version hosting the default `gcloud` and runs `gcloud components update -q --version $VERSION`. The new version will be stored in `~/.config/pincloud/versions/$VERSION`. This new directory will be an entire SDK, so it's possible that space will be an issue in some circumstances.

To manage versions, pincloud will have a few special commands that it recognizes, named to be extremely unlikely conflicts.

```
$ gcloud pincloud {install,remove} $VERSION
```

## Completion

Completion will "just work", as pincloud forwards the commands (and environment) in completion mode.
