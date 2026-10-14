/**
 * @Author: dn-jinmin
 * @File:  gogen
 * @Version: 1.0.0
 * @Date: 2024/4/16
 * @Description:
 */

package conf

import (
	"fmt"
	"path"
	"strings"

	"github.com/spf13/viper"
)

type Loadhandler func(string, any) error

var (
	loaders = map[string]Loadhandler{
		".yaml": LoadFromYamlBytes,
	}
)

func MustLoad(file string, v any) {
	Load(file, v)
}

// Load loads config into v from file, .json, .yaml and .yml are acceptable.
func Load(file string, v any) error {
	loader, ok := loaders[strings.ToLower(path.Ext(file))]
	if !ok {
		return fmt.Errorf("unrecognized file type: %s", file)
	}

	return loader(file, v)
}

func LoadFromYamlBytes(file string, v any) error {
	viper.SetConfigType("yaml")
	file = strings.Replace(file, "\\", "/", -1)
	viper.AddConfigPath(file[:strings.LastIndex(file, "/")+1])
	filename := file[strings.LastIndex(file, "/")+1 : strings.LastIndex(file, ".")]
	viper.SetConfigName(filename)

	if err := viper.ReadInConfig(); err != nil {
		return err
	}
	if err := viper.Unmarshal(v); err != nil {
		return err
	}

	return nil
}
