package essh

import (
	"fmt"
	"github.com/cjoudrey/gluahttp"
	"github.com/kohkimakimoto/gluaenv"
	"github.com/kohkimakimoto/gluafs"
	"github.com/kohkimakimoto/gluaquestion"
	"github.com/kohkimakimoto/gluatemplate"
	"github.com/kohkimakimoto/gluayaml"
	gluajson "layeh.com/gopher-json"
	"github.com/otm/gluash"
	"github.com/yuin/gluare"
	"github.com/yuin/gopher-lua"
	"net/http"
	"os"
	"path/filepath"
	"unicode"
)

func InitLuaState(L *lua.LState) {
	// custom type.
	registerTaskClass(L)
	registerHostClass(L)
	registerHostQueryClass(L)
	registerDriverClass(L)
	registerRegistryClass(L)
	registerJobClass(L)

	// global functions
	L.SetGlobal("host", L.NewFunction(esshHost))
	L.SetGlobal("task", L.NewFunction(esshTask))
	L.SetGlobal("driver", L.NewFunction(esshDriver))
	L.SetGlobal("job", L.NewFunction(esshJob))
	L.SetGlobal("import", L.NewFunction(esshImport))

	// modules
	L.PreloadModule("json", gluajson.Loader)
	L.PreloadModule("fs", gluafs.Loader)
	L.PreloadModule("yaml", gluayaml.Loader)
	L.PreloadModule("template", gluatemplate.Loader)
	L.PreloadModule("question", gluaquestion.Loader)
	L.PreloadModule("env", gluaenv.Loader)
	L.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)
	L.PreloadModule("re", gluare.Loader)
	L.PreloadModule("sh", gluash.Loader)

	// global variables
	lessh := L.NewTable()
	L.SetGlobal("essh", lessh)
	lessh.RawSetString("ssh_config", lua.LNil)
	lessh.RawSetString("version", lua.LString(Version))
	lessh.RawSetString("module", lua.LNil)

	L.SetFuncs(lessh, map[string]lua.LGFunction{
		// aliases global function.
		"host":   esshHost,
		"task":   esshTask,
		"driver": esshDriver,
		"job":    esshJob,
		"import": esshImport,

		// utility functions
		"debug":            esshDebug,
		"select_hosts":     esshSelectHosts,
		"jobs":             esshJobs,
		"get_job":          esshGetJob,
		"current_registry": esshCurrentRegistry,
	})
}

func esshDebug(L *lua.LState) int {
	msg := L.CheckString(1)
	if debugFlag {
		fmt.Printf("[essh debug] %s\n", msg)
	}

	return 0
}

func esshHost(L *lua.LState) int {
	name := L.CheckString(1)
	if L.GetTop() == 1 {
		// object or DSL style
		h := registerHost(L, name)
		L.Push(newLHost(L, h))

		return 1
	} else if L.GetTop() == 2 {
		// function style
		tb := L.CheckTable(2)
		h := registerHost(L, name)
		setupHost(L, h, tb)
		L.Push(newLHost(L, h))

		return 1
	}

	panic("host requires 1 or 2 arguments")
}

func esshTask(L *lua.LState) int {
	first := L.CheckAny(1)
	if tb, ok := toLTable(first); ok {
		name := DefaultTaskName
		j := registerTask(L, name)
		setupTask(L, j, tb)
		L.Push(newLTask(L, j))

		return 1
	}

	name := L.CheckString(1)
	if L.GetTop() == 1 {
		// object or DSL style
		t := registerTask(L, name)
		L.Push(newLTask(L, t))

		return 1
	} else if L.GetTop() == 2 {
		// function style
		tb := L.CheckTable(2)
		t := registerTask(L, name)
		setupTask(L, t, tb)
		L.Push(newLTask(L, t))

		return 1
	}

	panic("task requires 1 or 2 arguments")
}

func esshDriver(L *lua.LState) int {
	first := L.CheckAny(1)
	if tb, ok := toLTable(first); ok {
		name := DefaultDriverName
		d := registerDriver(L, name)
		setupDriver(L, d, tb)
		L.Push(newLDriver(L, d))

		return 1
	}

	name := L.CheckString(1)
	if L.GetTop() == 1 {
		// object or DSL style
		d := registerDriver(L, name)
		L.Push(newLDriver(L, d))

		return 1
	} else if L.GetTop() == 2 {
		// function style
		tb := L.CheckTable(2)
		d := registerDriver(L, name)
		setupDriver(L, d, tb)
		L.Push(newLDriver(L, d))

		return 1
	}

	panic("driver requires 1 or 2 arguments")
}

func esshJob(L *lua.LState) int {
	first := L.CheckAny(1)
	if tb, ok := toLTable(first); ok {
		name := DefaultJobName
		j := registerJob(L, name)
		setupJob(L, j, tb)
		L.Push(newLJob(L, j))

		return 1
	}

	name := L.CheckString(1)
	if L.GetTop() == 1 {
		// object or DSL style
		j := registerJob(L, name)
		L.Push(newLJob(L, j))

		return 1
	} else if L.GetTop() == 2 {
		// function style
		tb := L.CheckTable(2)
		j := registerJob(L, name)
		setupJob(L, j, tb)
		L.Push(newLJob(L, j))

		return 1
	}

	panic("job requires 1 or 2 arguments")
}

func esshImport(L *lua.LState) int {
	name := L.CheckString(1)
	lessh, ok := toLTable(L.GetGlobal("essh"))
	if !ok {
		L.RaiseError("'essh' global variable is broken")
	}
	mod := lessh.RawGetString("module")
	if mod != lua.LNil {
		L.RaiseError("'essh.module' is existed. does not support nested module importing.")
	}

	module := CurrentRegistry.LoadedModules[name]
	if module == nil {
		module = NewModule(name)

		update := updateFlag
		if CurrentRegistry.Type == RegistryTypeGlobal && !withGlobalFlag {
			update = false
		}

		err := module.Load(update)
		if err != nil {
			L.RaiseError("%v", err)
		}

		indexFile := module.IndexFile()
		if _, err := os.Stat(indexFile); err != nil {
			L.RaiseError("invalid module: %v", err)
		}

		// init module variable
		modulevar := L.NewTable()
		modulevar.RawSetString("path", lua.LString(filepath.Dir(indexFile)))
		modulevar.RawSetString("import_path", lua.LString(name))
		lessh.RawSetString("module", modulevar)

		if err := L.DoFile(indexFile); err != nil {
			panic(err)
		}
		// remove module variable
		lessh.RawSetString("module", lua.LNil)

		// get a module return value
		ret := L.Get(-1)
		module.Value = ret

		// register loaded module.
		CurrentRegistry.LoadedModules[name] = module

		return 1
	}

	L.Push(module.Value)
	return 1
}

func esshSelectHosts(L *lua.LState) int {
	hostQuery := NewHostQuery()

	if L.GetTop() > 2 {
		panic("select_hosts can receive max 2 argument.")
	}

	var job *Job

	first := L.Get(1)
	if ud, ok := first.(*lua.LUserData); ok {
		if v, ok := ud.Value.(*Job); ok {
			job = v
		} else {
			panic("expected a job but got an other userdata.")
		}
	}

	if L.GetTop() == 1 {
		if job == nil {
			value := L.CheckAny(1)
			selections := []string{}

			if selectionsStr, ok := toString(value); ok {
				selections = []string{selectionsStr}
			} else if selectionsSlice, ok := toSlice(value); ok {
				for _, selection := range selectionsSlice {
					if selectionStr, ok := selection.(string); ok {
						selections = append(selections, selectionStr)
					}
				}
			} else {
				panic("select_hosts can receive string or array table of strings.")
			}
			hostQuery.AppendSelections(selections)
		} else {
			hostQuery.SetDatasource(job.Hosts)
		}
	} else if L.GetTop() == 2 {
		if job != nil {
			value := L.CheckAny(2)
			selections := []string{}

			if selectionsStr, ok := toString(value); ok {
				selections = []string{selectionsStr}
			} else if selectionsSlice, ok := toSlice(value); ok {
				for _, selection := range selectionsSlice {
					if selectionStr, ok := selection.(string); ok {
						selections = append(selections, selectionStr)
					}
				}
			} else {
				panic("select_hosts can receive string or array table of strings.")
			}

			hostQuery.SetDatasource(job.Hosts).AppendSelections(selections)
		} else {
			panic("expected a job but got an other userdata.")
		}
	}
	L.Push(newLHostQuery(L, hostQuery))
	return 1
}

func esshJobs(L *lua.LState) int {
	tb := L.NewTable()
	for _, job := range Jobs {
		tb.Append(newLJob(L, job))
	}

	L.Push(tb)
	return 1
}

func esshGetJob(L *lua.LState) int {
	name := L.CheckString(1)
	job := Jobs[name]
	if job == nil {
		L.Push(lua.LNil)
		return 1
	}

	L.Push(newLJob(L, job))
	return 1
}

func registerHost(L *lua.LState, name string) *Host {
	if debugFlag {
		fmt.Printf("[essh debug] register host: %s\n", name)
	}

	h := NewHost()
	h.Name = name
	h.Registry = CurrentRegistry

	if host := Hosts[h.Name]; host != nil {
		// detect same name host
		h.Child = host
		host.Parent = h
	}

	Hosts[h.Name] = h

	return h
}

func registerTask(L *lua.LState, name string) *Task {
	if debugFlag {
		fmt.Printf("[essh debug] register task: %s\n", name)
	}

	t := NewTask()
	t.Name = name
	t.Registry = CurrentRegistry

	if task := Tasks[t.Name]; task != nil {
		// detect same name host
		t.Child = task
		task.Parent = t
	}

	Tasks[t.Name] = t

	return t
}

func registerDriver(L *lua.LState, name string) *Driver {
	if debugFlag {
		fmt.Printf("[essh debug] register driver: %s\n", name)
	}

	d := NewDriver()
	d.Name = name
	d.Registry = CurrentRegistry

	if driver := Drivers[d.Name]; driver != nil {
		// detect same name host
		d.Child = driver
		driver.Parent = d
	}

	Drivers[d.Name] = d

	return d
}

func registerJob(L *lua.LState, name string) *Job {
	if debugFlag {
		fmt.Printf("[essh debug] register job: %s\n", name)
	}

	j := NewJob()
	j.Name = name

	Jobs[j.Name] = j

	return j
}

func setupHost(L *lua.LState, h *Host, config *lua.LTable) {
	config.ForEach(func(k, v lua.LValue) {
		if kstr, ok := toString(k); ok {
			updateHost(L, h, kstr, v)
		}
	})
}

func updateHost(L *lua.LState, h *Host, key string, value lua.LValue) {
	h.LValues[key] = value

	var firstChar rune
	for _, c := range key {
		firstChar = c
		break
	}

	if unicode.IsUpper(firstChar) {
		if valuestr, ok := toString(value); ok {
			h.SSHConfig[key] = valuestr
			return
		}

		panic("SSH property must be string")
	}

	switch key {
	case "props":
		if propsTb, ok := toLTable(value); ok {
			// initialize
			h.Props = map[string]string{}

			propsTb.ForEach(func(propsKey lua.LValue, propsValue lua.LValue) {
				propsKeyStr, ok := toString(propsKey)
				if !ok {
					L.RaiseError("props table's key must be a string: %v", propsKey)
				}
				propsValueStr, ok := toString(propsValue)
				if !ok {
					L.RaiseError("props table's value must be a string: %v", propsValue)
				}

				h.Props[propsKeyStr] = propsValueStr
			})
		} else {
			panic("invalid value of a host's field '" + key + "'.")
		}
	case "hooks_before_connect":
		if tb, ok := toLTable(value); ok {
			maxn := tb.MaxN()
			hooks := make([]interface{}, 0, maxn)
			for i := 1; i <= maxn; i++ {
				hooks = append(hooks, toGoValue(tb.RawGetInt(i)))
			}

			h.HooksBeforeConnect = hooks
		} else {
			panic("invalid value of a host's field '" + key + "'.")
		}
	case "hooks_after_connect":
		if tb, ok := toLTable(value); ok {
			maxn := tb.MaxN()
			hooks := make([]interface{}, 0, maxn)
			for i := 1; i <= maxn; i++ {
				hooks = append(hooks, toGoValue(tb.RawGetInt(i)))
			}

			h.HooksAfterConnect = hooks
		} else {
			panic("invalid value of a host's field '" + key + "'.")
		}
	case "hooks_after_disconnect":
		if tb, ok := toLTable(value); ok {
			maxn := tb.MaxN()
			hooks := make([]interface{}, 0, maxn)
			for i := 1; i <= maxn; i++ {
				hooks = append(hooks, toGoValue(tb.RawGetInt(i)))
			}

			h.HooksAfterDisconnect = hooks
		} else {
			panic("invalid value of a host's field '" + key + "'.")
		}
	case "description":
		if descStr, ok := toString(value); ok {
			h.Description = descStr
		} else {
			panic("invalid value of a host's field '" + key + "'.")
		}

	case "hidden":
		if hiddenBool, ok := toBool(value); ok {
			h.Hidden = hiddenBool
		} else {
			panic("invalid value of a host's field '" + key + "'.")
		}

	case "tags":
		if tagsTb, ok := toLTable(value); ok {
			// initialize
			h.Tags = []string{}

			tagsTb.ForEach(func(_ lua.LValue, v lua.LValue) {
				if vs, ok := toString(v); ok {
					h.Tags = append(h.Tags, vs)
				} else {
					L.RaiseError("unsupported format of tags.")
				}
			})
		} else {
			panic("invalid value of a host's field '" + key + "'.")
		}

	default:
		panic("unsupported host's field '" + key + "'.")

	}
}

func toScript(L *lua.LState, value lua.LValue) ([]map[string]string, error) {
	ret := []map[string]string{}

	if tb, ok := toLTable(value); ok {
		maxn := tb.MaxN()
		if maxn == 0 { // table
			if tb.RawGetString("code") == lua.LNil {
				return nil, fmt.Errorf("if a 'script' entry is table, it has to have 'code' property.")
			}

			m := map[string]string{}
			tb.ForEach(func(k, v lua.LValue) {
				vs, ok := toString(v)
				if !ok {
					vb, ok := toBool(v)
					if !ok {
						panic("if a 'script' entry is table, it's value has to be string or bool.")
					}
					if vb {
						vs = "true"
					} else {
						vs = "false"
					}
				}
				ks, ok := toString(k)
				if !ok {
					panic("if a 'script' entry is table, it's property has to be string.")
				}
				m[ks] = vs
			})

			ret = append(ret, m)
		} else { // array
			for i := 1; i <= maxn; i++ {
				value := tb.RawGetInt(i)
				if fn, ok := toLFunction(value); ok {
					err := L.CallByParam(lua.P{
						Fn:      fn,
						NRet:    1,
						Protect: false,
					})
					if err != nil {
						panic(err)
					}
					funcRet := L.Get(-1)
					L.Pop(1)

					script, err := toScript(L, funcRet)
					if err != nil {
						return nil, err
					}
					ret = append(ret, script...)
				} else {
					script, err := toScript(L, value)
					if err != nil {
						return nil, err
					}
					ret = append(ret, script...)
				}
			}
		}
		return ret, nil
	} else if str, ok := toString(value); ok {
		return []map[string]string{
			map[string]string{"code": str},
		}, nil
	}

	return nil, fmt.Errorf("'script' got a invalid value.")
}

func esshCurrentRegistry(L *lua.LState) int {
	L.Push(newLRegistry(L, CurrentRegistry))
	return 1
}

// This code inspired by https://github.com/yuin/gluamapper/blob/master/gluamapper.go
func toGoValue(lv lua.LValue) interface{} {
	switch v := lv.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LString:
		return string(v)
	case lua.LNumber:
		return float64(v)
	case *lua.LTable:
		maxn := v.MaxN()
		if maxn == 0 { // table
			ret := make(map[string]interface{})
			v.ForEach(func(key, value lua.LValue) {
				keystr := fmt.Sprint(toGoValue(key))
				ret[keystr] = toGoValue(value)
			})
			return ret
		} else { // array
			ret := make([]interface{}, 0, maxn)
			for i := 1; i <= maxn; i++ {
				ret = append(ret, toGoValue(v.RawGetInt(i)))
			}
			return ret
		}
	default:
		return v
	}
}

func toBool(v lua.LValue) (bool, bool) {
	if lv, ok := v.(lua.LBool); ok {
		return bool(lv), true
	} else {
		return false, false
	}
}

func toString(v lua.LValue) (string, bool) {
	if lv, ok := v.(lua.LString); ok {
		return string(lv), true
	} else {
		return "", false
	}
}

func toMap(v lua.LValue) (map[string]interface{}, bool) {
	if lv, ok := toGoValue(v).(map[string]interface{}); ok {
		return lv, true
	} else {
		return nil, false
	}
}

func toSlice(v lua.LValue) ([]interface{}, bool) {
	gov := toGoValue(v)
	if lv, ok := gov.([]interface{}); ok {
		return lv, true
	} else if lv, ok := gov.(map[string]interface{}); ok {
		if len(lv) == 0 {
			return []interface{}{}, true
		}
		return nil, false
	} else {
		return nil, false
	}
}

func toLFunction(v lua.LValue) (*lua.LFunction, bool) {
	if lv, ok := v.(*lua.LFunction); ok {
		return lv, true
	} else {
		return nil, false
	}
}

func toLTable(v lua.LValue) (*lua.LTable, bool) {
	if lv, ok := v.(*lua.LTable); ok {
		return lv, true
	} else {
		return nil, false
	}
}

func toLUserData(v lua.LValue) (*lua.LUserData, bool) {
	if lv, ok := v.(*lua.LUserData); ok {
		return lv, true
	} else {
		return nil, false
	}
}

func toFloat64(v lua.LValue) (float64, bool) {
	if lv, ok := v.(lua.LNumber); ok {
		return float64(lv), true
	} else {
		return 0, false
	}
}

const LTaskClass = "Task*"

func registerTaskClass(L *lua.LState) {
	mt := L.NewTypeMetatable(LTaskClass)
	mt.RawSetString("__call", L.NewFunction(taskCall))
	mt.RawSetString("__index", L.NewFunction(taskIndex))
	mt.RawSetString("__newindex", L.NewFunction(taskNewindex))
}

func newLTask(L *lua.LState, task *Task) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = task
	L.SetMetatable(ud, L.GetTypeMetatable(LTaskClass))
	return ud
}

func checkTask(L *lua.LState) *Task {
	ud := L.CheckUserData(1)
	if v, ok := ud.Value.(*Task); ok {
		return v
	}
	L.ArgError(1, "Task object expected")
	return nil
}

func taskCall(L *lua.LState) int {
	task := checkTask(L)
	tb := L.CheckTable(2)

	setupTask(L, task, tb)

	L.Push(L.CheckUserData(1))
	return 1
}

func taskIndex(L *lua.LState) int {
	task := checkTask(L)
	index := L.CheckString(2)

	if index == "name" {
		L.Push(L.NewFunction(func(L *lua.LState) int {
			L.Push(lua.LString(task.Name))
			return 1
		}))
		return 1
	}

	v, ok := task.LValues[index]
	if v == nil || !ok {
		v = lua.LNil
	}

	L.Push(v)
	return 1
}

func taskNewindex(L *lua.LState) int {
	task := checkTask(L)
	index := L.CheckString(2)
	value := L.CheckAny(3)

	updateTask(L, task, index, value)

	return 0
}

func setupTask(L *lua.LState, t *Task, config *lua.LTable) {
	config.ForEach(func(k, v lua.LValue) {
		if kstr, ok := toString(k); ok {
			updateTask(L, t, kstr, v)
		}
	})
}

func updateTask(L *lua.LState, task *Task, key string, value lua.LValue) {
	task.LValues[key] = value

	switch key {
	case "backend":
		if backendStr, ok := toString(value); ok {
			task.Backend = backendStr
			if backendStr != TASK_BACKEND_LOCAL && backendStr != TASK_BACKEND_REMOTE {
				L.RaiseError("backend must be '%s' or '%s'.", TASK_BACKEND_LOCAL, TASK_BACKEND_REMOTE)
			}
		}
	case "targets":
		if targetsStr, ok := toString(value); ok {
			task.Targets = []string{targetsStr}
		} else if targetsSlice, ok := toSlice(value); ok {
			task.Targets = []string{}

			for _, target := range targetsSlice {
				if targetStr, ok := target.(string); ok {
					task.Targets = append(task.Targets, targetStr)
				}
			}
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "filters":
		if filtersStr, ok := toString(value); ok {
			task.Filters = []string{filtersStr}
		} else if filtersSlice, ok := toSlice(value); ok {
			task.Filters = []string{}

			for _, filter := range filtersSlice {
				if filterStr, ok := filter.(string); ok {
					task.Filters = append(task.Filters, filterStr)
				}
			}
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "description":
		if descStr, ok := toString(value); ok {
			task.Description = descStr
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "pty":
		if ptyBool, ok := toBool(value); ok {
			task.Pty = ptyBool
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "driver":
		if driverStr, ok := toString(value); ok {
			task.Driver = driverStr
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "parallel":
		if parallelBool, ok := toBool(value); ok {
			task.Parallel = parallelBool
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "privileged":
		if privilegedBool, ok := toBool(value); ok {
			task.Privileged = privilegedBool
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "disabled":
		if disabledBool, ok := toBool(value); ok {
			task.Disabled = disabledBool
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "hidden":
		if hiddenBool, ok := toBool(value); ok {
			task.Hidden = hiddenBool
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "script":
		script, err := toScript(L, value)
		if err != nil {
			L.RaiseError("%v", err)
		}
		task.Script = script

		if task.File != "" && len(task.Script) > 0 {
			L.RaiseError("invalid task definition: can't use 'script_file' and 'script' at the same time.")
		}
	case "script_file":
		if fileStr, ok := toString(value); ok {
			task.File = fileStr
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}

		if task.File != "" && len(task.Script) > 0 {
			L.RaiseError("invalid task definition: can't use 'script_file' and 'script' at the same time.")
		}
	case "prefix":
		if prefixBool, ok := toBool(value); ok {
			task.UsePrefix = prefixBool
		} else if prefixStr, ok := toString(value); ok {
			task.UsePrefix = true
			task.Prefix = prefixStr
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "prepare":
		if prepareFn, ok := value.(*lua.LFunction); ok {
			task.Prepare = func() error {
				err := L.CallByParam(lua.P{
					Fn:      prepareFn,
					NRet:    1,
					Protect: false,
				}, newLTask(L, task))
				if err != nil {
					return err
				}

				ret := L.Get(-1) // returned value
				L.Pop(1)

				if ret == lua.LNil {
					return nil
				} else if retB, ok := ret.(lua.LBool); ok {
					if retB {
						return nil
					} else {
						return fmt.Errorf("returned false from the prepare function.")
					}
				}

				return nil
			}
		} else {
			L.RaiseError("prepare have to be a function.")
		}
	case "props":
		if propsTb, ok := toLTable(value); ok {
			// initialize
			task.Props = map[string]string{}

			propsTb.ForEach(func(propsKey lua.LValue, propsValue lua.LValue) {
				propsKeyStr, ok := toString(propsKey)
				if !ok {
					L.RaiseError("props table's key must be a string: %v", propsKey)
				}
				propsValueStr, ok := toString(propsValue)
				if !ok {
					L.RaiseError("props table's value must be a string: %v", propsValue)
				}

				task.Props[propsKeyStr] = propsValueStr
			})
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	case "args":
		if argsSlice, ok := toSlice(value); ok {
			task.Args = []string{}

			for _, arg := range argsSlice {
				if argStr, ok := arg.(string); ok {
					task.Args = append(task.Args, argStr)
				}
			}
		} else {
			panic("invalid value of a task's field '" + key + "'.")
		}
	default:
		panic("unsupported task's field '" + key + "'.")
	}
}

const LHostClass = "Host*"

func registerHostClass(L *lua.LState) {
	mt := L.NewTypeMetatable(LHostClass)
	mt.RawSetString("__call", L.NewFunction(hostCall))
	mt.RawSetString("__index", L.NewFunction(hostIndex))
	mt.RawSetString("__newindex", L.NewFunction(hostNewindex))
}

func newLHost(L *lua.LState, host *Host) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = host
	L.SetMetatable(ud, L.GetTypeMetatable(LHostClass))
	return ud
}

func checkHost(L *lua.LState) *Host {
	ud := L.CheckUserData(1)
	if v, ok := ud.Value.(*Host); ok {
		return v
	}
	L.ArgError(1, "Host object expected")
	return nil
}

func hostCall(L *lua.LState) int {
	host := checkHost(L)
	tb := L.CheckTable(2)

	setupHost(L, host, tb)

	L.Push(L.CheckUserData(1))
	return 1
}

func hostIndex(L *lua.LState) int {
	host := checkHost(L)
	index := L.CheckString(2)

	if index == "name" {
		L.Push(L.NewFunction(func(L *lua.LState) int {
			L.Push(lua.LString(host.Name))
			return 1
		}))
		return 1
	}

	v, ok := host.LValues[index]
	if v == nil || !ok {
		v = lua.LNil
	}

	L.Push(v)
	return 1
}

func hostNewindex(L *lua.LState) int {
	host := checkHost(L)
	index := L.CheckString(2)
	value := L.CheckAny(3)

	updateHost(L, host, index, value)

	return 0
}

const LHostQueryClass = "HostQuery*"

func registerHostQueryClass(L *lua.LState) {
	mt := L.NewTypeMetatable(LHostQueryClass)
	mt.RawSetString("__index", L.NewFunction(hostQueryIndex))
}

func newLHostQuery(L *lua.LState, hostQuery *HostQuery) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = hostQuery
	L.SetMetatable(ud, L.GetTypeMetatable(LHostQueryClass))
	return ud
}

func checkHostQuery(L *lua.LState) *HostQuery {
	ud := L.CheckUserData(1)
	if v, ok := ud.Value.(*HostQuery); ok {
		return v
	}
	L.ArgError(1, "HostQuery object expected")
	return nil
}

func hostQueryIndex(L *lua.LState) int {
	//_ := checkHostQuery(L)
	//_ := L.CheckUserData(1)
	index := L.CheckString(2)

	switch index {
	case "filter":
		L.Push(L.NewFunction(func(L *lua.LState) int {
			hostQuery := checkHostQuery(L)
			ud := L.CheckUserData(1)
			if L.GetTop() != 2 {
				panic("filter must receive max 2 argument.")
			} else {
				filters := []string{}
				value := L.CheckAny(2)
				if filtersStr, ok := toString(value); ok {
					filters = []string{filtersStr}
				} else if filtersSlice, ok := toSlice(value); ok {
					for _, filter := range filtersSlice {
						if filterStr, ok := filter.(string); ok {
							filters = append(filters, filterStr)
						}
					}
				} else {
					panic("filter can receive string or array table of strings.")
				}

				hostQuery.AppendFilters(filters)
			}

			ud.Value = hostQuery
			L.Push(ud)
			return 1
		}))

		return 1
	case "get":
		L.Push(L.NewFunction(func(L *lua.LState) int {
			hostQuery := checkHostQuery(L)

			lhosts := L.NewTable()
			for _, host := range hostQuery.GetHosts() {
				lhost := newLHost(L, host)
				lhosts.Append(lhost)
			}

			L.Push(lhosts)
			return 1
		}))

		return 1
	case "first":
		L.Push(L.NewFunction(func(L *lua.LState) int {
			L.Push(L.NewFunction(func(L *lua.LState) int {
				hostQuery := checkHostQuery(L)

				hosts := hostQuery.GetHosts()
				if len(hosts) > 0 {
					L.Push(newLHost(L, hosts[0]))
					return 1
				}
				L.Push(lua.LNil)
				return 1
			}))
			return 1
		}))
		L.Push(lua.LNil)
		return 1
	default:
		L.Push(lua.LNil)
		return 1
	}
}

const LDriverClass = "Driver*"

func registerDriverClass(L *lua.LState) {
	mt := L.NewTypeMetatable(LDriverClass)
	mt.RawSetString("__call", L.NewFunction(driverCall))
	mt.RawSetString("__index", L.NewFunction(driverIndex))
	mt.RawSetString("__newindex", L.NewFunction(driverNewindex))
}

func newLDriver(L *lua.LState, driver *Driver) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = driver
	L.SetMetatable(ud, L.GetTypeMetatable(LDriverClass))
	return ud
}

func checkDriver(L *lua.LState) *Driver {
	ud := L.CheckUserData(1)
	if v, ok := ud.Value.(*Driver); ok {
		return v
	}
	L.ArgError(1, "Driver object expected")
	return nil
}

func driverCall(L *lua.LState) int {
	driver := checkDriver(L)
	tb := L.CheckTable(2)

	setupDriver(L, driver, tb)

	L.Push(L.CheckUserData(1))
	return 1
}

func driverIndex(L *lua.LState) int {
	driver := checkDriver(L)
	index := L.CheckString(2)

	if index == "name" {
		L.Push(L.NewFunction(func(L *lua.LState) int {
			L.Push(lua.LString(driver.Name))
			return 1
		}))
		return 1
	}

	v, ok := driver.LValues[index]
	if v == nil || !ok {
		v = lua.LNil
	}

	L.Push(v)
	return 1
}

func driverNewindex(L *lua.LState) int {
	driver := checkDriver(L)
	index := L.CheckString(2)
	value := L.CheckAny(3)

	updateDriver(L, driver, index, value)

	return 0
}

func setupDriver(L *lua.LState, driver *Driver, config *lua.LTable) {
	config.ForEach(func(k, v lua.LValue) {
		if kstr, ok := toString(k); ok {
			updateDriver(L, driver, kstr, v)
		}
	})
}

func updateDriver(L *lua.LState, driver *Driver, key string, value lua.LValue) {
	driver.LValues[key] = value

	switch key {
	case "engine":
		if engineFn, ok := value.(*lua.LFunction); ok {
			driver.Engine = func(driver *Driver) (string, error) {
				err := L.CallByParam(lua.P{
					Fn:      engineFn,
					NRet:    1,
					Protect: true,
				}, newLDriver(L, driver))
				if err != nil {
					return "", err
				}

				ret := L.Get(-1) // returned value
				L.Pop(1)

				if retStr, ok := toString(ret); ok {
					return retStr, nil
				} else {
					return "", fmt.Errorf("driver engine has to return a string.")
				}
			}
		} else if engineStr, ok := toString(value); ok {
			driver.Engine = func(driver *Driver) (string, error) {
				return engineStr, nil
			}
		} else {
			L.RaiseError("driver 'engine' have to be a function or string.")
		}
	}
}

const LRegistryClass = "Registry*"

func newLRegistry(L *lua.LState, ctx *Registry) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = ctx
	L.SetMetatable(ud, L.GetTypeMetatable(LRegistryClass))
	return ud
}

func checkRegistry(L *lua.LState) *Registry {
	ud := L.CheckUserData(1)
	if v, ok := ud.Value.(*Registry); ok {
		return v
	}
	L.ArgError(1, "Registry object expected")
	return nil
}

func registerRegistryClass(L *lua.LState) {
	mt := L.NewTypeMetatable(LRegistryClass)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"data_dir":    registryDataDir,
		"tmp_dir":     registryTmpDir,
		"modules_dir": registryModulesDir,
		"type":        registryType,
	}))
}

func registryDataDir(L *lua.LState) int {
	reg := checkRegistry(L)
	L.Push(lua.LString(reg.DataDir))
	return 1
}

func registryTmpDir(L *lua.LState) int {
	reg := checkRegistry(L)
	L.Push(lua.LString(reg.TmpDir()))
	return 1
}

func registryModulesDir(L *lua.LState) int {
	reg := checkRegistry(L)
	L.Push(lua.LString(reg.ModulesDir()))
	return 1
}

func registryType(L *lua.LState) int {
	reg := checkRegistry(L)
	L.Push(lua.LString(reg.TypeString()))
	return 1
}

const LJobClass = "Job*"

func registerJobClass(L *lua.LState) {
	mt := L.NewTypeMetatable(LJobClass)
	mt.RawSetString("__call", L.NewFunction(jobCall))
	mt.RawSetString("__index", L.NewFunction(jobIndex))
	mt.RawSetString("__newindex", L.NewFunction(jobNewindex))
}

func newLJob(L *lua.LState, job *Job) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = job
	L.SetMetatable(ud, L.GetTypeMetatable(LJobClass))
	return ud
}

func checkJob(L *lua.LState) *Job {
	ud := L.CheckUserData(1)
	if v, ok := ud.Value.(*Job); ok {
		return v
	}
	L.ArgError(1, "Job object expected")
	return nil
}

func jobCall(L *lua.LState) int {
	job := checkJob(L)
	tb := L.CheckTable(2)

	setupJob(L, job, tb)

	return 0
}

func jobIndex(L *lua.LState) int {
	job := checkJob(L)
	index := L.CheckString(2)

	switch index {
	case "name":
		L.Push(L.NewFunction(func(L *lua.LState) int {
			L.Push(lua.LString(job.Name))
			return 1
		}))
	case "select_hosts":
		L.Push(L.NewFunction(esshSelectHosts))
	default:
		v, ok := job.LValues[index]
		if v == nil || !ok {
			v = lua.LNil
		}
		L.Push(v)
	}

	return 1
}

func jobNewindex(L *lua.LState) int {
	job := checkJob(L)
	index := L.CheckString(2)
	value := L.CheckAny(3)

	updateJob(L, job, index, value)

	return 0
}

func setupJob(L *lua.LState, job *Job, config *lua.LTable) {
	// guarantee evaluating a key/value dictionary at first.
	config.ForEach(func(k, v lua.LValue) {
		if kstr, ok := toString(k); ok {
			updateJob(L, job, kstr, v)
		}
	})

	config.ForEach(func(k, v lua.LValue) {
		if _, ok := toString(k); ok {
			return
		} else if _, ok := toFloat64(k); ok {
			// set a host, task or driver
			lv, ok := v.(*lua.LUserData)
			if !ok {
				panic(fmt.Sprintf("expected userdata (host, task or driver) but got '%v'\n", v))
			}

			switch resource := lv.Value.(type) {
			case *Host:
				// set host table data
				if job.LValues["hosts"] == nil {
					job.LValues["hosts"] = L.NewTable()
				}
				hosts, ok := toLTable(job.LValues["hosts"])
				if !ok {
					panic("broken 'hosts' table")
				}
				host := L.NewTable()
				resource.MapLValuesToLTable(host)
				hosts.RawSetString(resource.Name, host)

				// register host object
				job.RegisterHost(resource)
			case *Task:
				// set task table data
				if job.LValues["tasks"] == nil {
					job.LValues["tasks"] = L.NewTable()
				}
				tasks, ok := toLTable(job.LValues["tasks"])
				if !ok {
					panic("broken 'tasks' table")
				}
				task := L.NewTable()
				resource.MapLValuesToLTable(task)
				tasks.RawSetString(resource.Name, task)

				// register task object
				job.RegisterTask(resource)
			case *Driver:
				// set task table data
				if job.LValues["drivers"] == nil {
					job.LValues["drivers"] = L.NewTable()
				}
				drivers, ok := toLTable(job.LValues["drivers"])
				if !ok {
					panic("broken 'drivers' table")
				}
				driver := L.NewTable()
				resource.MapLValuesToLTable(driver)
				drivers.RawSetString(resource.Name, driver)

				// register task object
				job.RegisterDriver(resource)
			default:
				panic(fmt.Sprintf("expected host, task or driver but got '%v'\n", resource))
			}
		} else {
			panic("invalid operation\n")
		}
	})
}

func updateJob(L *lua.LState, job *Job, key string, value lua.LValue) {
	job.LValues[key] = value

	switch key {
	//case "base":
	//	if tb, ok := toLTable(value); ok {
	//		setupJob(L, job, tb)
	//	} else {
	//		panic(fmt.Sprintf("expected table but got '%v'\n", value))
	//	}
	case "description":
		if descStr, ok := toString(value); ok {
			job.Description = descStr
		} else {
			panic("invalid value of a job's field '" + key + "'.")
		}
	case "hidden":
		if hiddenBool, ok := toBool(value); ok {
			job.Hidden = hiddenBool
		} else {
			panic("invalid value of a job's field '" + key + "'.")
		}
	case "prepare":
		if prepareFn, ok := value.(*lua.LFunction); ok {
			job.Prepare = func() error {
				err := L.CallByParam(lua.P{
					Fn:      prepareFn,
					NRet:    1,
					Protect: false,
				}, newLJob(L, job))
				if err != nil {
					return err
				}

				ret := L.Get(-1) // returned value
				L.Pop(1)

				if ret == lua.LNil {
					return nil
				} else if retB, ok := ret.(lua.LBool); ok {
					if retB {
						return nil
					} else {
						return fmt.Errorf("returned false from the prepare function.")
					}
				}

				return nil
			}
		} else {
			L.RaiseError("prepare have to be a function.")
		}

	//case "props":
	//	if propsTb, ok := toLTable(value); ok {
	//		// initialize
	//		job.Props = map[string]string{}
	//
	//		propsTb.ForEach(func(propsKey lua.LValue, propsValue lua.LValue) {
	//			propsKeyStr, ok := toString(propsKey)
	//			if !ok {
	//				L.RaiseError("props table's key must be a string: %v", propsKey)
	//			}
	//			propsValueStr, ok := toString(propsValue)
	//			if !ok {
	//				L.RaiseError("props table's value must be a string: %v", propsValue)
	//			}
	//
	//			job.Props[propsKeyStr] = propsValueStr
	//		})
	//	} else {
	//		panic("invalid value of a job's field '" + key + "'.")
	//	}
	case "hosts":
		if tb, ok := toLTable(value); ok {
			// initialize
			job.Hosts = map[string]*Host{}

			tb.ForEach(func(k, v lua.LValue) {
				name, ok := toString(k)
				if !ok {
					panic(fmt.Sprintf("expected string of host's name but got '%v'\n", k))
				}

				config, ok := toLTable(v)
				if !ok {
					panic(fmt.Sprintf("expected table of host's config but got '%v'\n", v))
				}

				h := registerHost(L, name)
				setupHost(L, h, config)
				job.RegisterHost(h)
			})
		} else {
			panic(fmt.Sprintf("expected table but got '%v'\n", value))
		}
	case "tasks":
		if tb, ok := toLTable(value); ok {
			// initialize
			job.Tasks = map[string]*Task{}

			tb.ForEach(func(k, v lua.LValue) {
				name, ok := toString(k)
				if !ok {
					panic(fmt.Sprintf("expected string of task's name but got '%v'\n", k))
				}

				config, ok := toLTable(v)
				if !ok {
					panic(fmt.Sprintf("expected table of task's config but got '%v'\n", v))
				}

				t := registerTask(L, name)
				setupTask(L, t, config)
				job.RegisterTask(t)
			})
		} else {
			panic(fmt.Sprintf("expected table but got '%v'\n", value))
		}
	case "drivers":
		if tb, ok := toLTable(value); ok {
			// initialize
			job.Drivers = map[string]*Driver{
				DefaultDriverName: DefaultDriver,
			}

			tb.ForEach(func(k, v lua.LValue) {
				name, ok := toString(k)
				if !ok {
					panic(fmt.Sprintf("expected string of driver's name but got '%v'\n", k))
				}

				config, ok := toLTable(v)
				if !ok {
					panic(fmt.Sprintf("expected table of driver's config but got '%v'\n", v))
				}

				d := registerDriver(L, name)
				setupDriver(L, d, config)
				job.RegisterDriver(d)
			})
		} else {
			panic(fmt.Sprintf("expected table but got '%v'\n", value))
		}
	default:
		panic("unsupported job's field '" + key + "'.")
	}

}
