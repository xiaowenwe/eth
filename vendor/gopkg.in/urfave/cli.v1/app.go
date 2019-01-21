package cli

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"time"
)

var (
	changeLogURL                    = "https://github.com/urfave/cli/blob/master/CHANGELOG.md"
	appActionDeprecationURL         = fmt.Sprintf("%s#deprecated-cli-app-action-signature", changeLogURL)
	runAndExitOnErrorDeprecationURL = fmt.Sprintf("%s#deprecated-cli-app-runandexitonerror", changeLogURL)

	contactSysadmin = "This is an error in the application.  Please contact the distributor of this application if this is not you."

	errInvalidActionType = NewExitError("ERROR invalid Action type. "+
		fmt.Sprintf("Must be `func(*Context`)` or `func(*Context) error).  %s", contactSysadmin)+
		fmt.Sprintf("See %s", appActionDeprecationURL), 2)
)

// App是cli应用程序的主要结构。 这是建议
//使用cli.NewApp（）函数创建一个应用程序
// App is the main structure of a cli application. It is recommended that
// an app be created with the cli.NewApp() function
type App struct {
	// The name of the program. Defaults to path.Base(os.Args[0])/程序名称。默认为path.base(os.Args[0])
	Name string
	// Full name of command for help, defaults to Name/命令全名用于帮助，默认为名称
	HelpName string
	// Description of the program.//描述程序。
	Usage string
	// Text to override the USAGE section of help/Text覆盖“帮助”的“使用”部分
	UsageText string
	// Description of the program argument format.//描述程序参数格式。
	ArgsUsage string
	// Version of the program/程序的版本
	Version string
	// Description of the program/程序说明
	Description string
	// List of commands to execute/要执行的命令列表
	Commands []Command
	// List of flags to parse/要解析的标志列表
	Flags []Flag
	// Boolean to enable bash completion commands/boole以启用bash完成命令
	EnableBashCompletion bool
	// Boolean to hide built-in help command/boole来隐藏内置的帮助命令
	HideHelp bool
	// Boolean to hide built-in version flag and the VERSION section of help
	HideVersion bool
	// Populate on app startup, only gettable through method Categories()/在应用程序启动时填充，只能通过方法类别()获取
	categories CommandCategories
	// An action to execute when the bash-completion flag is set/设置bash完成标志时执行的操作。
	BashComplete BashCompleteFunc
	// An action to execute before any subcommands are run, but after the context is ready
	// If a non-nil error is returned, no subcommands are run/在运行任何子命令之前执行的操作，但在上下文准备就绪/如果返回非零错误时，不运行任何子命令。
	Before BeforeFunc
	// An action to execute after any subcommands are run, but after the subcommand has finished
	// It is run even if Action() panics/在运行任何子命令之后执行的操作，但是在子命令完成/即使Action()恐慌时也会运行它
	After AfterFunc
	//如果没有指定/期望一个‘cli.ActionFunc’，但将接受‘func(*cli.Context){}’/*注释*的*废弃*签名时执行的操作：将在以后的版本中删除对已废弃的‘Action’签名的支持。
	// The action to execute when no subcommands are specified
	// Expects a `cli.ActionFunc` but will accept the *deprecated* signature of `func(*cli.Context) {}`
	// *Note*: support for the deprecated `Action` signature will be removed in a future version
	Action interface{}

	// Execute this function if the proper command cannot be found/如果找不到正确的命令，则执行此函数
	CommandNotFound CommandNotFoundFunc
	// Execute this function if an usage error occurs/如果发生使用错误，则执行此函数
	OnUsageError OnUsageErrorFunc
	// Compilation date
	Compiled time.Time
	// List of all authors who contributed/所有撰稿人名单
	Authors []Author
	// Copyright of the binary if any/二进制文件的版权(如果有的话)
	Copyright string
	// Name of Author (Note: Use App.Authors, this is deprecated)/作者姓名(注：使用App.Authors，这是不推荐的)
	Author string
	// Email of Author (Note: Use App.Authors, this is deprecated)/作者的电子邮件(注：请使用App.Authors，这是不推荐的)
	Email string
	// Writer writer to write output to将输出写入/Writer编写器
	Writer io.Writer
	// ErrWriter writes error outputerrwriter输出写入错误
	ErrWriter io.Writer
	// Other custom info其他自定义信息
	Metadata map[string]interface{}
	// Carries a function which returns app specific info./带有返回应用程序特定信息的函数。
	ExtraInfo func() map[string]string
	//CustomAppHelpTemplate应用程序帮助主题的文本模板./cli.go使用文本/模板来呈现模板。可以通过设置此变量来/呈现自定义帮助文本。
	// CustomAppHelpTemplate the text template for app help topic.
	// cli.go uses text/template to render templates. You can
	// render custom help text by setting this variable.
	CustomAppHelpTemplate string

	didSetup bool
}

//试图找出这个二进制文件是何时编译的。/如果它找不到，返回当前时间。
// Tries to find out when this binary was compiled.
// Returns the current time if it fails to find it.
func compileTime() time.Time {
	info, err := os.Stat(os.Args[0])
	if err != nil {
		return time.Now()
	}
	return info.ModTime()
}

//NewApp创建了一个新的CLI应用程序，其名称、/使用、版本和操作都有一些合理的默认值。
// NewApp creates a new cli Application with some reasonable defaults for Name,
// Usage, Version and Action.
func NewApp() *App {
	return &App{
		Name:         filepath.Base(os.Args[0]),
		HelpName:     filepath.Base(os.Args[0]),
		Usage:        "A new cli application",
		UsageText:    "",
		Version:      "0.0.0",
		BashComplete: DefaultAppComplete,
		Action:       helpCommand.Action,
		Compiled:     compileTime(),
		Writer:       os.Stdout,
	}
}

//安装程序运行初始化代码，以确保所有数据结构在“运行”之前都已准备好进行/`Run‘或检查。它由“Run”内部调用，但如果安装已经发生，则会提前返回。
// Setup runs initialization code to ensure all data structures are ready for
// `Run` or inspection prior to `Run`.  It is internally called by `Run`, but
// will return early if setup has already happened.
func (a *App) Setup() {
	if a.didSetup {
		return
	}

	a.didSetup = true

	if a.Author != "" || a.Email != "" {
		a.Authors = append(a.Authors, Author{Name: a.Author, Email: a.Email})
	}

	newCmds := []Command{}
	for _, c := range a.Commands {
		if c.HelpName == "" {
			c.HelpName = fmt.Sprintf("%s %s", a.HelpName, c.Name)
		}
		newCmds = append(newCmds, c)
	}
	a.Commands = newCmds

	if a.Command(helpCommand.Name) == nil && !a.HideHelp {
		a.Commands = append(a.Commands, helpCommand)
		if (HelpFlag != BoolFlag{}) {
			a.appendFlag(HelpFlag)
		}
	}

	if !a.HideVersion {
		a.appendFlag(VersionFlag)
	}

	a.categories = CommandCategories{}
	for _, command := range a.Commands {
		a.categories = a.categories.AddCommand(command.Category, command)
	}
	sort.Sort(a.categories)

	if a.Metadata == nil {
		a.Metadata = make(map[string]interface{})
	}

	if a.Writer == nil {
		a.Writer = os.Stdout
	}
}

//run是cli应用程序的入口点。解析参数片段和路由/到正确的标志/args组合。
// Run is the entry point to the cli app. Parses the arguments slice and routes
// to the proper flag/args combination
func (a *App) Run(arguments []string) (err error) {
	a.Setup()
	//将完成标志与标记集分开处理，因为可以在标记之后尝试完成，
	//但在将其值放在命令行之前。这导致标记集将完成标志名称解释为其前面标志的值，这是不可取的，请注意，我们只能这样做，因为shell自动完成函数总是在命令末尾追加完成标志。
	// handle the completion flag separately from the flagset since
	// completion could be attempted after a flag, but before its value was put
	// on the command line. this causes the flagset to interpret the completion
	// flag name as the value of the flag before it which is undesirable
	// note that we can only do this because the shell autocomplete function
	// always appends the completion flag at the end of the command
	shellComplete, arguments := checkShellCompleteFlag(a, arguments)

	// parse flags
	set, err := flagSet(a.Name, a.Flags)
	if err != nil {
		return err
	}

	set.SetOutput(ioutil.Discard)
	err = set.Parse(arguments[1:])
	nerr := normalizeFlags(a.Flags, set)
	context := NewContext(a, set, nil)
	if nerr != nil {
		fmt.Fprintln(a.Writer, nerr)
		ShowAppHelp(context)
		return nerr
	}
	context.shellComplete = shellComplete

	if checkCompletions(context) {
		return nil
	}

	if err != nil {
		if a.OnUsageError != nil {
			err := a.OnUsageError(context, err, false)
			HandleExitCoder(err)
			return err
		}
		fmt.Fprintf(a.Writer, "%s %s\n\n", "Incorrect Usage.", err.Error())
		ShowAppHelp(context)
		return err
	}

	if !a.HideHelp && checkHelp(context) {
		ShowAppHelp(context)
		return nil
	}

	if !a.HideVersion && checkVersion(context) {
		ShowVersion(context)
		return nil
	}

	if a.After != nil {
		defer func() {
			if afterErr := a.After(context); afterErr != nil {
				if err != nil {
					err = NewMultiError(err, afterErr)
				} else {
					err = afterErr
				}
			}
		}()
	}

	if a.Before != nil {
		beforeErr := a.Before(context)
		if beforeErr != nil {
			ShowAppHelp(context)
			HandleExitCoder(beforeErr)
			err = beforeErr
			return err
		}
	}

	args := context.Args()
	if args.Present() {
		name := args.First()
		c := a.Command(name)
		if c != nil {
			return c.Run(context)
		}
	}

	if a.Action == nil {
		a.Action = helpCommand.Action
	}

	// Run default Action
	err = HandleAction(a.Action, context)

	HandleExitCoder(err)
	return err
}

//RunAndExitOnError调用.Run()并在返回/弃用错误时退出非零：相反，您应该返回一个完成cli.ExitCoder/to cli.App.Run的错误。
// 这将导致应用程序在cli.ExitCoder中使用给定的eror/代码退出。
// RunAndExitOnError calls .Run() and exits non-zero if an error was returned
//
// Deprecated: instead you should return an error that fulfills cli.ExitCoder
// to cli.App.Run. This will cause the application to exit with the given eror
// code in the cli.ExitCoder
func (a *App) RunAndExitOnError() {
	if err := a.Run(os.Args); err != nil {
		fmt.Fprintln(a.errWriter(), err)
		OsExiter(1)
	}
}

//RunAsSub命令调用给定上下文的子命令，解析ctx.args()到/Generate命令特定的标志
// RunAsSubcommand invokes the subcommand given the context, parses ctx.Args() to
// generate command-specific flags
func (a *App) RunAsSubcommand(ctx *Context) (err error) {
	// append help to commands
	if len(a.Commands) > 0 {
		if a.Command(helpCommand.Name) == nil && !a.HideHelp {
			a.Commands = append(a.Commands, helpCommand)
			if (HelpFlag != BoolFlag{}) {
				a.appendFlag(HelpFlag)
			}
		}
	}

	newCmds := []Command{}
	for _, c := range a.Commands {
		if c.HelpName == "" {
			c.HelpName = fmt.Sprintf("%s %s", a.HelpName, c.Name)
		}
		newCmds = append(newCmds, c)
	}
	a.Commands = newCmds

	// parse flags
	set, err := flagSet(a.Name, a.Flags)
	if err != nil {
		return err
	}

	set.SetOutput(ioutil.Discard)
	err = set.Parse(ctx.Args().Tail())
	nerr := normalizeFlags(a.Flags, set)
	context := NewContext(a, set, ctx)

	if nerr != nil {
		fmt.Fprintln(a.Writer, nerr)
		fmt.Fprintln(a.Writer)
		if len(a.Commands) > 0 {
			ShowSubcommandHelp(context)
		} else {
			ShowCommandHelp(ctx, context.Args().First())
		}
		return nerr
	}

	if checkCompletions(context) {
		return nil
	}

	if err != nil {
		if a.OnUsageError != nil {
			err = a.OnUsageError(context, err, true)
			HandleExitCoder(err)
			return err
		}
		fmt.Fprintf(a.Writer, "%s %s\n\n", "Incorrect Usage.", err.Error())
		ShowSubcommandHelp(context)
		return err
	}

	if len(a.Commands) > 0 {
		if checkSubcommandHelp(context) {
			return nil
		}
	} else {
		if checkCommandHelp(ctx, context.Args().First()) {
			return nil
		}
	}

	if a.After != nil {
		defer func() {
			afterErr := a.After(context)
			if afterErr != nil {
				HandleExitCoder(err)
				if err != nil {
					err = NewMultiError(err, afterErr)
				} else {
					err = afterErr
				}
			}
		}()
	}

	if a.Before != nil {
		beforeErr := a.Before(context)
		if beforeErr != nil {
			HandleExitCoder(beforeErr)
			err = beforeErr
			return err
		}
	}

	args := context.Args()
	if args.Present() {
		name := args.First()
		c := a.Command(name)
		if c != nil {
			return c.Run(context)
		}
	}

	// Run default Action
	err = HandleAction(a.Action, context)

	HandleExitCoder(err)
	return err
}

//Command在App上返回指定的命令。如果该命令不存在，则返回零。
// Command returns the named command on App. Returns nil if the command does not exist
func (a *App) Command(name string) *Command {
	for _, c := range a.Commands {
		if c.HasName(name) {
			return &c
		}
	}

	return nil
}

//类别返回一个包含所有类别及其包含的命令的片段
// Categories returns a slice containing all the categories with the commands they contain
func (a *App) Categories() CommandCategories {
	return a.categories
}

//VisibleCollection返回隐藏=false的类别和命令的片段
// VisibleCategories returns a slice of categories and commands that are
// Hidden=false
func (a *App) VisibleCategories() []*CommandCategory {
	ret := []*CommandCategory{}
	for _, category := range a.categories {
		if visible := func() *CommandCategory {
			for _, command := range category.Commands {
				if !command.Hidden {
					return category
				}
			}
			return nil
		}(); visible != nil {
			ret = append(ret, visible)
		}
	}
	return ret
}

//VisibleCommands返回带有Hidden=false的命令片段
// VisibleCommands returns a slice of the Commands with Hidden=false
func (a *App) VisibleCommands() []Command {
	ret := []Command{}
	for _, command := range a.Commands {
		if !command.Hidden {
			ret = append(ret, command)
		}
	}
	return ret
}

//VisibleFlags返回带Hidden=false的FlagesofFlageswithHidden=false
// VisibleFlags returns a slice of the Flags with Hidden=false
func (a *App) VisibleFlags() []Flag {
	return visibleFlags(a.Flags)
}

func (a *App) hasFlag(flag Flag) bool {
	for _, f := range a.Flags {
		if flag == f {
			return true
		}
	}

	return false
}

func (a *App) errWriter() io.Writer {
	//当应用程序ErrWriter为0时，使用包级别1。
	// When the app ErrWriter is nil use the package level one.
	if a.ErrWriter == nil {
		return ErrWriter
	}

	return a.ErrWriter
}

func (a *App) appendFlag(flag Flag) {
	if !a.hasFlag(flag) {
		a.Flags = append(a.Flags, flag)
	}
}

//Author代表为CLI项目做出贡献的人。
// Author represents someone who has contributed to a cli project.
type Author struct {
	Name  string // The Authors name
	Email string // The Authors email
}

//string使作者遵守Stringer接口，以便在模板过程中方便地打印
// String makes Author comply to the Stringer interface, to allow an easy print in the templating process
func (a Author) String() string {
	e := ""
	if a.Email != "" {
		e = " <" + a.Email + ">"
	}

	return fmt.Sprintf("%v%v", a.Name, e)
}

//HandleAction试图找出使用了哪一个Action签名。如果/是ActionFunc或带有ActionFunc遗留签名的func，那么func/将运行
// HandleAction attempts to figure out which Action signature was used.  If
// it's an ActionFunc or a func with the legacy signature for Action, the func
// is run!
func HandleAction(action interface{}, context *Context) (err error) {
	if a, ok := action.(ActionFunc); ok {
		return a(context)
	} else if a, ok := action.(func(*Context) error); ok {
		return a(context)
	} else if a, ok := action.(func(*Context)); ok { // deprecated function signature
		a(context)
		return nil
	} else {
		return errInvalidActionType
	}
}
