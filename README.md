# Configmanager
Configmanager is the what mixpanel uses to add dynamic configuration to our code across several services. 

Configmanager does these things:
* Reads a file `configs.json` under a specified path, and watches it for changes
* Interprets the data in the file and gives an easy interface to access the configuration values 

Here is a possible example for a hypothetical service called my-service

```go
  cm, err := configmanager.NewClient("/etc/configs", "my-service", fr)
  if err != nil {
  	return err
  }
  defer cm.Close()
  if cm.GetBoolean("my-feature-enabled", false) {
  	// do the my-feature thing
  }
```
The only requirement for this example to work is placing marshalled configurations in the file `/etc/configs/my-service/configs.json` 

## How are configs written
Mixpanel writes configs using [JSONNET](https://jsonnet.org/) and they are mounted 
using kubernetes configmaps. However configmanager only cares about the format
and a file that follows the path `/dir-path/[name of config group]/configs.json`

Two key things you need to unerstand are:
* Config group: A group of configs that belong together in same scope, because they are used by same feature or same service etc
* Config: A key, value pair where value can be an arbitrary JSON and you call configmanager methods specifiying the key

A config group marshalled in a file looks like this:
```
[
  {
    "key": "feature_enabled_customers",
    "value": {
      "123": {},
      "456": {}
    }
  },
  {
    "key": "scaling_percentage",
    "value": 1
  },
  {
    "key": "timeout_secs",
    "value": 5
  }
]
```
If such a config is placed in the file `/etc/configs/my-configs/configs.json` then the configmanager
will be constructed using `configmanager.NewClient("/etc/configs", "my-configs", fr)`
