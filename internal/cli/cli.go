package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/pkdiv/uplift/internal/config"
	"github.com/pkdiv/uplift/internal/confirmation"
	"github.com/pkdiv/uplift/internal/output"
	"github.com/pkdiv/uplift/internal/tui"
	"github.com/pkdiv/uplift/internal/uploader"
)

type App struct {
	ConfigPath string
	In         io.Reader
	Out        io.Writer
	ErrOut     io.Writer
	HomeDir    bool
	FilesList  string
}

func New(configPath string, in io.Reader, out io.Writer, errOut io.Writer) *App {
	return &App{ConfigPath: configPath, In: in, Out: out, ErrOut: errOut}
}

func NewWithHome(configPath string, in io.Reader, out io.Writer, errOut io.Writer, homeDir bool) *App {
	return &App{ConfigPath: configPath, In: in, Out: out, ErrOut: errOut, HomeDir: homeDir}
}

func (a *App) Run(args []string) int {
	if len(args) == 0 {
		return a.runTUI()
	}

	switch args[0] {
	case "upload":
		return a.runUpload(args[1:])
	case "profile":
		return a.runProfile(args[1:])
	case "help", "-h", "--help":
		a.usage()
		return 0
	default:
		fmt.Fprintf(a.ErrOut, "unknown command %q\n\n", args[0])
		a.usage()
		return 2
	}
}

func (a *App) runTUI() int {
	cfg, err := a.loadConfig()
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	if err := tui.Run(cfg, a.ConfigPath, a.HomeDir, a.FilesList); err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) usage() {
	fmt.Fprint(a.Out, `SFTP Upload - upload files to a remote server over SFTP

Usage:
  uplift upload <profile> [--concurrency N] <files...>
  uplift profile add
  uplift profile list
  uplift profile show <name>
  uplift profile edit <name>
  uplift profile remove <name>

Global flags:
  --config <path>    Path to configuration file
`)
}

func readFileList(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read files list %s: %w", path, err)
	}

	var files []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		files = append(files, line)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("files list %s is empty", path)
	}
	return files, nil
}

func (a *App) loadConfig() (*config.Config, error) {
	return config.Load(a.ConfigPath)
}

func (a *App) saveConfig(cfg *config.Config) error {
	return cfg.Save(a.ConfigPath)
}

func (a *App) runUpload(args []string) int {
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(a.ErrOut)
	concurrency := fs.Int("concurrency", 1, "maximum number of simultaneous uploads")
	filesList := fs.String("files-list", "", "path to a text file containing relative file paths, one per line")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	remaining := fs.Args()
	if len(remaining) < 2 {
		fmt.Fprintln(a.ErrOut, "usage: uplift upload <profile> [--concurrency N] [--files-list file.txt] <files...>")
		return 2
	}

	if *concurrency < 1 {
		fmt.Fprintln(a.ErrOut, "--concurrency must be at least 1")
		return 2
	}

	profileName := remaining[0]
	files := remaining[1:]

	if *filesList != "" {
		listFiles, err := readFileList(*filesList)
		if err != nil {
			fmt.Fprintf(a.ErrOut, "error: %v\n", err)
			return 1
		}
		files = append(files, listFiles...)
	}

	if len(files) == 0 {
		fmt.Fprintln(a.ErrOut, "error: no files specified")
		return 2
	}

	cfg, err := a.loadConfig()
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	profile, ok := cfg.Get(profileName)
	if !ok {
		fmt.Fprintf(a.ErrOut, "error: profile %q not found\n", profileName)
		return 1
	}

	writer := output.New(a.Out)
	prompter := confirmation.New(a.In, a.Out)

	writer.Header("SFTP Upload")
	writer.Section("Profile", profileName)
	writer.Section("Server", fmt.Sprintf("%s@%s:%d", profile.Username, profile.Host, profile.Port))
	writer.Section("Local base", profile.LocalBase)
	writer.Section("Remote base", profile.RemoteBase)
	writer.Section("Concurrency", strconv.Itoa(*concurrency))
	writer.Section("Files", strconv.Itoa(len(files)))
	writer.BlankLine()

	up := uploader.New(profile, writer)
	jobs, err := up.Resolve(files)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	results, err := up.Upload(jobs, uploader.Options{
		Concurrency: *concurrency,
		CreateRemote: true,
		Confirm: func(job uploader.Job) (bool, error) {
			writer.BlankLine()
			writer.Printf("File %d/%d\n\n", job.Index, job.Total)
			writer.Section("Local", job.LocalPath)
			writer.Section("Remote", job.RemotePath)
			return prompter.Ask("Upload this file?")
		},
		OverwriteCheck: func(remotePath string) (bool, error) {
			writer.BlankLine()
			writer.Println("Remote file already exists:")
			writer.BlankLine()
			writer.Println("  " + remotePath)
			writer.BlankLine()
			return prompter.Ask("Overwrite?")
		},
	})
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	uploaded, skipped, failed := up.Summarize(results)

	writer.BlankLine()
	writer.Header("Upload complete")
	writer.Section("Uploaded", strconv.Itoa(uploaded))
	writer.Section("Skipped", strconv.Itoa(skipped))
	writer.Section("Failed", strconv.Itoa(failed))
	writer.Section("Profile", profileName)
	writer.Section("Concurrency", strconv.Itoa(*concurrency))

	if failed > 0 {
		writer.BlankLine()
		writer.Println("Failed files:")
		writer.BlankLine()
		for _, r := range results {
			if !r.Success && !r.Skipped {
				writer.Printf("  %s\n    → %s\n    %v\n\n", r.Job.LocalPath, r.Job.RemotePath, r.Err)
			}
		}
		return 1
	}

	return 0
}

func (a *App) runProfile(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.ErrOut, "usage: uplift profile <add|list|show|edit|remove> [name]")
		return 2
	}

	switch args[0] {
	case "add":
		return a.profileAdd()
	case "list":
		return a.profileList()
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(a.ErrOut, "usage: uplift profile show <name>")
			return 2
		}
		return a.profileShow(args[1])
	case "edit":
		if len(args) < 2 {
			fmt.Fprintln(a.ErrOut, "usage: uplift profile edit <name>")
			return 2
		}
		return a.profileEdit(args[1])
	case "remove":
		if len(args) < 2 {
			fmt.Fprintln(a.ErrOut, "usage: uplift profile remove <name>")
			return 2
		}
		return a.profileRemove(args[1])
	default:
		fmt.Fprintf(a.ErrOut, "unknown profile command %q\n", args[0])
		return 2
	}
}

func (a *App) profileAdd() int {
	cfg, err := a.loadConfig()
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	prompter := confirmation.New(a.In, a.Out)
	writer := output.New(a.Out)

	writer.Header("Create SFTP upload profile")

	name, err := prompter.AskStringDefault("Profile name", "")
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}
	if err := config.ValidateProfileName(name); err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	host, err := prompter.AskStringDefault("Host", "")
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	port, err := prompter.AskIntDefault("Port", 22)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	username, err := prompter.AskStringDefault("Username", "")
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	keyFile, err := prompter.AskStringDefault("Private key", "")
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	localBase, err := prompter.AskStringDefault("Local base path", "")
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	remoteBase, err := prompter.AskStringDefault("Remote base path", "")
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	profile := config.Profile{
		Host:       host,
		Port:       port,
		Username:   username,
		KeyFile:    keyFile,
		LocalBase:  localBase,
		RemoteBase: remoteBase,
	}
	profile.Normalize()

	if err := config.ValidateProfile(profile); err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	writer.BlankLine()
	writer.Section("Profile", name)
	writer.Section("Server", fmt.Sprintf("%s:%d", profile.Host, profile.Port))
	writer.Section("Username", profile.Username)
	writer.Section("Private key", profile.KeyFile)
	writer.Section("Local base", profile.LocalBase)
	writer.Section("Remote base", profile.RemoteBase)
	writer.BlankLine()

	save, err := prompter.Ask("Save this profile?")
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}
	if !save {
		writer.Println("Profile not saved.")
		return 0
	}

	if err := cfg.Add(name, profile); err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}
	if err := a.saveConfig(cfg); err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	writer.Printf("✓ Profile %q saved.\n", name)
	return 0
}

func (a *App) profileList() int {
	cfg, err := a.loadConfig()
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	names := cfg.Names()
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Fprintln(a.Out, "No profiles configured.")
		return 0
	}

	fmt.Fprintln(a.Out, "NAME          HOST                    LOCAL BASE")
	for _, name := range names {
		p := cfg.Profiles[name]
		host := p.Host
		if p.Port != 0 && p.Port != 22 {
			host = fmt.Sprintf("%s:%d", p.Host, p.Port)
		}
		fmt.Fprintf(a.Out, "%-13s %-23s %s\n", name, host, p.LocalBase)
	}
	return 0
}

func (a *App) profileShow(name string) int {
	cfg, err := a.loadConfig()
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	p, ok := cfg.Get(name)
	if !ok {
		fmt.Fprintf(a.ErrOut, "error: profile %q not found\n", name)
		return 1
	}

	writer := output.New(a.Out)
	writer.Section("Profile", name)
	writer.Section("Host", fmt.Sprintf("%s:%d", p.Host, p.Port))
	writer.Section("Username", p.Username)
	writer.Section("Private key", p.KeyFile)
	writer.Section("Local base", p.LocalBase)
	writer.Section("Remote base", p.RemoteBase)
	return 0
}

func (a *App) profileEdit(name string) int {
	cfg, err := a.loadConfig()
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	p, ok := cfg.Get(name)
	if !ok {
		fmt.Fprintf(a.ErrOut, "error: profile %q not found\n", name)
		return 1
	}

	prompter := confirmation.New(a.In, a.Out)
	writer := output.New(a.Out)

	writer.Printf("Edit profile: %s\n\n", name)

	host, err := prompter.AskStringDefault("Host", p.Host)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	port, err := prompter.AskIntDefault("Port", p.Port)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	username, err := prompter.AskStringDefault("Username", p.Username)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	keyFile, err := prompter.AskStringDefault("Private key", p.KeyFile)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	localBase, err := prompter.AskStringDefault("Local base", p.LocalBase)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	remoteBase, err := prompter.AskStringDefault("Remote base", p.RemoteBase)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	newProfile := config.Profile{
		Host:       host,
		Port:       port,
		Username:   username,
		KeyFile:    keyFile,
		LocalBase:  localBase,
		RemoteBase: remoteBase,
	}
	newProfile.Normalize()

	if err := config.ValidateProfile(newProfile); err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	writer.BlankLine()
	writer.Section("Profile", name)
	writer.Section("Server", fmt.Sprintf("%s:%d", newProfile.Host, newProfile.Port))
	writer.Section("Username", newProfile.Username)
	writer.Section("Private key", newProfile.KeyFile)
	writer.Section("Local base", newProfile.LocalBase)
	writer.Section("Remote base", newProfile.RemoteBase)
	writer.BlankLine()

	save, err := prompter.Ask("Save changes to profile '" + name + "'?")
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}
	if !save {
		writer.Println("Profile unchanged.")
		return 0
	}

	if err := cfg.Update(name, newProfile); err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}
	if err := a.saveConfig(cfg); err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	writer.Printf("✓ Profile %q updated.\n", name)
	return 0
}

func (a *App) profileRemove(name string) int {
	cfg, err := a.loadConfig()
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	if _, ok := cfg.Get(name); !ok {
		fmt.Fprintf(a.ErrOut, "error: profile %q not found\n", name)
		return 1
	}

	prompter := confirmation.New(a.In, a.Out)
	confirmed, err := prompter.Ask(fmt.Sprintf("Remove profile %q?", name))
	if err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}
	if !confirmed {
		fmt.Fprintln(a.Out, "Profile not removed.")
		return 0
	}

	if err := cfg.Remove(name); err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}
	if err := a.saveConfig(cfg); err != nil {
		fmt.Fprintf(a.ErrOut, "error: %v\n", err)
		return 1
	}

	fmt.Fprintf(a.Out, "✓ Profile %q removed.\n", name)
	return 0
}


