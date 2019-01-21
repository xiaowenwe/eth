package log

import (
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"sync"

	"github.com/go-stack/stack"
)

//Logger通过向Handler写入来打印其日志记录。/Handler接口定义了日志记录的写入位置和方式。
// /处理程序是可组合的，在组合/它们以实现适合您的应用程序的日志记录结构方面提供了极大的灵活性。
// A Logger prints its log records by writing to a Handler.
// The Handler interface defines where and how log records are written.
// Handlers are composable, providing you great flexibility in combining
// them to achieve the logging structure that suits your applications.
type Handler interface {
	Log(r *Record) error
}

//FuncHandler返回一个用给定/函数记录的Handler。
// FuncHandler returns a Handler that logs records with the given
// function.
func FuncHandler(fn func(r *Record) error) Handler {
	return funcHandler(fn)
}

type funcHandler func(r *Record) error

func (h funcHandler) Log(r *Record) error {
	return h(r)
}

//StreamHandler以给定的格式将日志记录写入io.Writer/。StreamHandler可以用来/轻松地开始向其他/输出写入日志记录。/
// StreamHandler用LazyHandler和SyncHandler/包装自己，以评估Lazy对象并执行安全并发写入。
// StreamHandler writes log records to an io.Writer
// with the given format. StreamHandler can be used
// to easily begin writing log records to other
// outputs.
//
// StreamHandler wraps itself with LazyHandler and SyncHandler
// to evaluate Lazy objects and perform safe concurrent writes.
func StreamHandler(wr io.Writer, fmtr Format) Handler {
	h := FuncHandler(func(r *Record) error {
		_, err := wr.Write(fmtr.Format(r))
		return err
	})
	return LazyHandler(SyncHandler(h))
}

//StreamHandler以给定的格式将日志记录写入io.Writer/。StreamHandler可以用来/轻松地开始向其他/输出写入日志记录。
// StreamHandler用LazyHandler和SyncHandler/包装自己，以评估Lazy对象并执行安全并发写入。
// SyncHandler can be wrapped around a handler to guarantee that
// only a single Log operation can proceed at a time. It's necessary
// for thread-safe concurrent writes.
func SyncHandler(h Handler) Handler {
	var mu sync.Mutex
	return FuncHandler(func(r *Record) error {
		defer mu.Unlock()
		mu.Lock()
		return h.Log(r)
	})
}

//FileHandler返回一个处理程序，该处理程序使用给定的格式将日志记录写入给定文件
// 。如果路径/已经存在，FileHandler将追加到给定文件。如果没有，/FileHandler将创建模式0644的文件。
// FileHandler returns a handler which writes log records to the give file
// using the given format. If the path
// already exists, FileHandler will append to the given file. If it does not,
// FileHandler will create the file with mode 0644.
func FileHandler(path string, fmtr Format) (Handler, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return closingHandler{f, StreamHandler(f, fmtr)}, nil
}

//NetHandler打开到给定地址的套接字，并将记录/写入连接。
// NetHandler opens a socket to the given address and writes records
// over the connection.
func NetHandler(network, addr string, fmtr Format) (Handler, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	return closingHandler{conn, StreamHandler(conn, fmtr)}, nil
}

///XXX：ClosingHandler目前基本上是未使用的/它是指在Handler接口支持/可能的CLOSE()操作的未来时间
// XXX: closingHandler is essentially unused at the moment
// it's meant for a future time when the Handler interface supports
// a possible Close() operation
type closingHandler struct {
	io.WriteCloser
	Handler
}

func (h *closingHandler) Close() error {
	return h.WriteCloser.Close()
}

//CallerFileHandler返回一个Handler，该处理程序将/调用函数的行号和文件添加到具有“调用方”键的上下文中。
// CallerFileHandler returns a Handler that adds the line number and file of
// the calling function to the context with key "caller".
func CallerFileHandler(h Handler) Handler {
	return FuncHandler(func(r *Record) error {
		r.Ctx = append(r.Ctx, "caller", fmt.Sprint(r.Call))
		return h.Log(r)
	})
}

//CaleFrunChHANDROR返回一个将调用函数名添加到
//上下文与键“FN”。

// CallerFuncHandler returns a Handler that adds the calling function name to
// the context with key "fn".
func CallerFuncHandler(h Handler) Handler {
	return FuncHandler(func(r *Record) error {
		r.Ctx = append(r.Ctx, "fn", formatCall("%+n", r.Call))
		return h.Log(r)
	})
}

//这个函数在这里是为了请您继续检查Go<1.8。
// This function is here to please go vet on Go < 1.8.
func formatCall(format string, c stack.Call) string {
	return fmt.Sprintf(format, c)
}

//CallerStackHandler返回一个Handler，它向上下文/添加一个堆栈跟踪，并使用键“堆栈”。
// 堆栈跟踪被格式化为匹配[]中/调用站点的空格分隔列表。首先列出最新的呼叫站点。
// /每个呼叫站点是根据格式化的。有关支持的格式列表，请参阅/Packagegithub.com/go-堆栈/堆栈的文档。
// CallerStackHandler returns a Handler that adds a stack trace to the context
// with key "stack". The stack trace is formated as a space separated list of
// call sites inside matching []'s. The most recent call site is listed first.
// Each call site is formatted according to format. See the documentation of
// package github.com/go-stack/stack for the list of supported formats.
func CallerStackHandler(format string, h Handler) Handler {
	return FuncHandler(func(r *Record) error {
		s := stack.Trace().TrimBelow(r.Call).TrimRuntime()
		if len(s) > 0 {
			r.Ctx = append(r.Ctx, "stack", fmt.Sprintf(format, s))
		}
		return h.Log(r)
	})
}

//FilterHandler返回一个Handler，该处理程序只在给定函数计算为true时将记录写入/包装处理程序。
// 例如，/只记录‘err’键不是nil：/logger.etHandler(FilterHandler(func(r*Record)
// bool{/for i：=0；i<len(r.Ctx)；i=2{/if r.ctx[i]=“err”{/返回r.ctx[i 1]！=nil/}/返回false/}，h)/
// FilterHandler returns a Handler that only writes records to the
// wrapped Handler if the given function evaluates true. For example,
// to only log records where the 'err' key is not nil:
//
//    logger.SetHandler(FilterHandler(func(r *Record) bool {
//        for i := 0; i < len(r.Ctx); i += 2 {
//            if r.Ctx[i] == "err" {
//                return r.Ctx[i+1] != nil
//            }
//        }
//        return false
//    }, h))
//
func FilterHandler(fn func(r *Record) bool, h Handler) Handler {
	return FuncHandler(func(r *Record) error {
		if fn(r) {
			return h.Log(r)
		}
		return nil
	})
}

//MatchFilterHandler返回一个Handler，该处理程序只在日志/上下文中的给定键与值匹配时将记录/写入包装处理程序。
// 例如，只记录/来自您的UI包：/log.MatchFilterHandler(“pkg”，“app/ui”，log.StdoutHandler)/
// MatchFilterHandler returns a Handler that only writes records
// to the wrapped Handler if the given key in the logged
// context matches the value. For example, to only log records
// from your ui package:
//
//    log.MatchFilterHandler("pkg", "app/ui", log.StdoutHandler)
//
func MatchFilterHandler(key string, value interface{}, h Handler) Handler {
	return FilterHandler(func(r *Record) (pass bool) {
		switch key {
		case r.KeyNames.Lvl:
			return r.Lvl == value
		case r.KeyNames.Time:
			return r.Time == value
		case r.KeyNames.Msg:
			return r.Msg == value
		}

		for i := 0; i < len(r.Ctx); i += 2 {
			if r.Ctx[i] == key {
				return r.Ctx[i+1] == value
			}
		}
		return false
	}, h)
}

//LvlFilterHandler返回一个Handler，该处理程序只向包装的Handler写入/记录，
// 这些记录小于给定的详细/级别。例如，只对/log错误/Crit记录：/log.LvlFilterHandler(log.LvlError，log.StdoutHandler)/
// LvlFilterHandler returns a Handler that only writes
// records which are less than the given verbosity
// level to the wrapped Handler. For example, to only
// log Error/Crit records:
//
//     log.LvlFilterHandler(log.LvlError, log.StdoutHandler)
//
func LvlFilterHandler(maxLvl Lvl, h Handler) Handler {
	return FilterHandler(func(r *Record) (pass bool) {
		return r.Lvl <= maxLvl
	}, h)
}

//MultiHandler向其每个处理程序分派任何写操作。/这对于将不同类型的日志信息/写入不同的位置非常有用。
// 例如，登录到文件和/标准错误：/log.MultiHandler(/log.Must.FileHandler(“/var/log/app.log”)、log.LogfmtFormat()、/log.StderrHandler)/
// A MultiHandler dispatches any write to each of its handlers.
// This is useful for writing different types of log information
// to different locations. For example, to log to a file and
// standard error:
//
//     log.MultiHandler(
//         log.Must.FileHandler("/var/log/app.log", log.LogfmtFormat()),
//         log.StderrHandler)
//
func MultiHandler(hs ...Handler) Handler {
	return FuncHandler(func(r *Record) error {
		for _, h := range hs {
			// what to do about failures?
			h.Log(r)
		}
		return nil
	})
}

//FailoverHandler将所有日志记录写入第一个处理程序/指定，但如果/第一个处理程序失败，则将故障转移和写入第二个处理程序，等等。
// /例如，您可能希望登录到网络套接字，但如果网络失败，则失败/写入文件，如果文件写入，则为/标准输出。
// 失败：/log.FailoverHandler(/log.Must.NetHandler(“TCP”、“：9090”、log.JsonFormat()、/log.Must.FileHandler(“/var/log/app.log”、log.LogfmtFormat()、/log.StdoutHandler)/所有未到第一个处理程序的写入程序都会添加/表单“over_err_{IDX}”的关键字，这将解释在/时遇到的错误。试图在列表中的处理程序之前写入它们。
// A FailoverHandler writes all log records to the first handler
// specified, but will failover and write to the second handler if
// the first handler has failed, and so on for all handlers specified.
// For example you might want to log to a network socket, but failover
// to writing to a file if the network fails, and then to
// standard out if the file write fails:
//
//     log.FailoverHandler(
//         log.Must.NetHandler("tcp", ":9090", log.JsonFormat()),
//         log.Must.FileHandler("/var/log/app.log", log.LogfmtFormat()),
//         log.StdoutHandler)
//
// All writes that do not go to the first handler will add context with keys of
// the form "failover_err_{idx}" which explain the error encountered while
// trying to write to the handlers before them in the list.
func FailoverHandler(hs ...Handler) Handler {
	return FuncHandler(func(r *Record) error {
		var err error
		for i, h := range hs {
			err = h.Log(r)
			if err == nil {
				return nil
			} else {
				r.Ctx = append(r.Ctx, fmt.Sprintf("failover_err_%d", i), err)
			}
		}

		return err
	})
}

//ChannelHandler将所有记录写入给定信道。/如果信道已满，则阻塞。对于日志消息的异步处理/非常有用，BufferedHandler使用它。
// ChannelHandler writes all records to the given channel.
// It blocks if the channel is full. Useful for async processing
// of log messages, it's used by BufferedHandler.
func ChannelHandler(recs chan<- *Record) Handler {
	return FuncHandler(func(r *Record) error {
		recs <- r
		return nil
	})
}

//BufferedHandler将所有记录写入具有给定大小的缓冲/通道，只要可以写入/处理程序，该通道就会刷新到包装/处理程序中。
// 由于这些/写是异步发生的，所以对BufferedHandler/从不返回错误的所有写操作都会被忽略。
// BufferedHandler writes all records to a buffered
// channel of the given size which flushes into the wrapped
// handler whenever it is available for writing. Since these
// writes happen asynchronously, all writes to a BufferedHandler
// never return an error and any errors from the wrapped handler are ignored.
func BufferedHandler(bufSize int, h Handler) Handler {
	recs := make(chan *Record, bufSize)
	go func() {
		for m := range recs {
			_ = h.Log(m)
		}
	}()
	return ChannelHandler(recs)
}

//LazyHandler在记录的上下文中评估/任何延迟函数之后，将所有值写入包装处理程序。
// 它已经包装在这个库中的StreamHandler和SyAdd.1-Handler周围，只有在编写自己的Handler时才需要/它。
// LazyHandler writes all values to the wrapped handler after evaluating
// any lazy functions in the record's context. It is already wrapped
// around StreamHandler and SyslogHandler in this library, you'll only need
// it if you write your own Handler.
func LazyHandler(h Handler) Handler {
	return FuncHandler(func(r *Record) error {
		//遍历值(奇数索引)，并将任何延迟的fn的值重新分配到其执行结果
		// go through the values (odd indices) and reassign
		// the values of any lazy fn to the result of its execution
		hadErr := false
		for i := 1; i < len(r.Ctx); i += 2 {
			lz, ok := r.Ctx[i].(Lazy)
			if ok {
				v, err := evaluateLazy(lz)
				if err != nil {
					hadErr = true
					r.Ctx[i] = err
				} else {
					if cs, ok := v.(stack.CallStack); ok {
						v = cs.TrimBelow(r.Call).TrimRuntime()
					}
					r.Ctx[i] = v
				}
			}
		}

		if hadErr {
			r.Ctx = append(r.Ctx, errorKey, "bad lazy")
		}

		return h.Log(r)
	})
}

func evaluateLazy(lz Lazy) (interface{}, error) {
	t := reflect.TypeOf(lz.Fn)

	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("INVALID_LAZY, not func: %+v", lz.Fn)
	}

	if t.NumIn() > 0 {
		return nil, fmt.Errorf("INVALID_LAZY, func takes args: %+v", lz.Fn)
	}

	if t.NumOut() == 0 {
		return nil, fmt.Errorf("INVALID_LAZY, no func return val: %+v", lz.Fn)
	}

	value := reflect.ValueOf(lz.Fn)
	results := value.Call([]reflect.Value{})
	if len(results) == 1 {
		return results[0].Interface(), nil
	} else {
		values := make([]interface{}, len(results))
		for i, v := range results {
			values[i] = v.Interface()
		}
		return values, nil
	}
}

//DishardHandler报告所有写入的成功，但不做任何操作。/它对于通过/Logger的SetHandler方法在运行时动态禁用日志非常有用。
// DiscardHandler reports success for all writes but does nothing.
// It is useful for dynamically disabling logging at runtime via
// a Logger's SetHandler method.
func DiscardHandler() Handler {
	return FuncHandler(func(r *Record) error {
		return nil
	})
}

//Object对象提供以下Handler创建函数/，该函数/不返回错误参数，而是在失败时返回Handler/和恐慌：FileHandler、NetHandler、SyAdd.1-Handler、SyAdd.1-NetHandler
// The Must object provides the following Handler creation functions
// which instead of returning an error parameter only return a Handler
// and panic on failure: FileHandler, NetHandler, SyslogHandler, SyslogNetHandler
var Must muster

func must(h Handler, err error) Handler {
	if err != nil {
		panic(err)
	}
	return h
}

type muster struct{}

func (m muster) FileHandler(path string, fmtr Format) Handler {
	return must(FileHandler(path, fmtr))
}

func (m muster) NetHandler(network, addr string, fmtr Format) Handler {
	return must(NetHandler(network, addr, fmtr))
}
