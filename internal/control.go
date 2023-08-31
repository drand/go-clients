package drand

import (
	"fmt"
	"github.com/drand/drand/common/log"
	json "github.com/nikkolasg/hexjson"
	"github.com/urfave/cli/v2"
	"io"
)

func schemesCmd(c *cli.Context, l log.Logger) error {
	client, err := controlClient(c, l)
	if err != nil {
		return err
	}

	resp, err := client.ListSchemes()
	if err != nil {
		return fmt.Errorf("drand: can't get the list of scheme ids availables ... %w", err)
	}

	fmt.Fprintf(c.App.Writer, "Drand supports the following list of schemes: \n")

	for i, id := range resp.Ids {
		fmt.Fprintf(c.App.Writer, "%d) %s \n", i, id)
	}

	fmt.Fprintf(c.App.Writer, "\nChoose one of them and set it on --%s flag \n", schemeFlag.Name)
	return nil
}

func printJSON(w io.Writer, j interface{}) error {
	buff, err := json.MarshalIndent(j, "", "    ")
	if err != nil {
		return fmt.Errorf("could not JSON marshal: %w", err)
	}
	fmt.Fprintln(w, string(buff))
	return nil
}
