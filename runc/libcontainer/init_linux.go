package libcontainer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/containerd/console"
	"github.com/moby/sys/user"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/opencontainers/runc/libcontainer/capabilities"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runc/libcontainer/system"
	"github.com/opencontainers/runc/libcontainer/utils"
)

type initType string

const (
	initSetns    initType = "setns"
	initStandard initType = "standard"
)

type pid struct {
	Pid           int `json:"stage2_pid"`
	PidFirstChild int `json:"stage1_pid"`
}

// network is an internal struct used to setup container networks.
type network struct {
	configs.Network

	// TempVethPeerName is a unique temporary veth peer name that was placed into
	// the container's namespace.
	TempVethPeerName string `json:"temp_veth_peer_name"`
}

type mountFds struct {
	// sourceFds are the fds to use as source when mounting.
	// The slice size should be the same as container mounts, as it will be
	// paired with them.
	// The value -1 is used when no fd is needed for the mount.
	// Can't have a valid fd in the same position that other slices in this struct.
	// We need to use only one of these fds on any single mount.
	sourceFds []int
	// Idem sourceFds, but fds of already created idmap mounts, to use with unix.MoveMount().
	idmapFds []int
}

// initConfig is used for transferring parameters from Exec() to Init()
type initConfig struct {
	Args             []string              `json:"args"`
	Env              []string              `json:"env"`
	Cwd              string                `json:"cwd"`
	Capabilities     *configs.Capabilities `json:"capabilities"`
	ProcessLabel     string                `json:"process_label"`
	AppArmorProfile  string                `json:"apparmor_profile"`
	NoNewPrivileges  bool                  `json:"no_new_privileges"`
	User             string                `json:"user"`
	AdditionalGroups []string              `json:"additional_groups"`
	Config           *configs.Config       `json:"config"`
	Networks         []*network            `json:"network"`
	PassedFilesCount int                   `json:"passed_files_count"`
	ContainerID      string                `json:"containerid"`
	Rlimits          []configs.Rlimit      `json:"rlimits"`
	CreateConsole    bool                  `json:"create_console"`
	ConsoleWidth     uint16                `json:"console_width"`
	ConsoleHeight    uint16                `json:"console_height"`
	RootlessEUID     bool                  `json:"rootless_euid,omitempty"`
	RootlessCgroups  bool                  `json:"rootless_cgroups,omitempty"`
	SpecState        *specs.State          `json:"spec_state,omitempty"`
	Cgroup2Path      string                `json:"cgroup2_path,omitempty"`
}

// Init is part of "runc init" implementation.
func Init() {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()

	if err := startInitialization(); err != nil {
		// If the error is returned, it was not communicated
		// back to the parent (which is not a common case),
		// so print it to stderr here as a last resort.
		//
		// Do not use logrus as we are not sure if it has been
		// set up yet, but most important, if the parent is
		// alive (and its log forwarding is working).
		fmt.Fprintln(os.Stderr, err)
	}
	// Normally, StartInitialization() never returns, meaning
	// if we are here, it had failed.
	os.Exit(1)
}

// Normally, this function does not return. If it returns, with or without an
// error, it means the initialization has failed. If the error is returned,
// it means the error can not be communicated back to the parent.
func startInitialization() (retErr error) {
	// Get the synchronisation pipe.
	envSyncPipe := os.Getenv("_LIBCONTAINER_SYNCPIPE")
	syncPipeFd, err := strconv.Atoi(envSyncPipe)
	if err != nil {
		return fmt.Errorf("unable to convert _LIBCONTAINER_SYNCPIPE: %w", err)
	}
	syncPipe := newSyncSocket(os.NewFile(uintptr(syncPipeFd), "sync"))
	defer syncPipe.Close()

	defer func() {
		// If this defer is ever called, this means initialization has failed.
		// Send the error back to the parent process in the form of an initError.
		ierr := initError{Message: retErr.Error()}
		if err := writeSyncArg(syncPipe, procError, ierr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		// The error is sent, no need to also return it (or it will be reported twice).
		retErr = nil
	}()

	// Get the INITPIPE.
	envInitPipe := os.Getenv("_LIBCONTAINER_INITPIPE")
	initPipeFd, err := strconv.Atoi(envInitPipe)
	if err != nil {
		return fmt.Errorf("unable to convert _LIBCONTAINER_INITPIPE: %w", err)
	}
	initPipe := os.NewFile(uintptr(initPipeFd), "init")
	defer initPipe.Close()

	// Set up logging. This is used rarely, and mostly for init debugging.

	// Passing log level is optional; currently libcontainer/integration does not do it.
	if levelStr := os.Getenv("_LIBCONTAINER_LOGLEVEL"); levelStr != "" {
		logLevel, err := strconv.Atoi(levelStr)
		if err != nil {
			return fmt.Errorf("unable to convert _LIBCONTAINER_LOGLEVEL: %w", err)
		}
		logrus.SetLevel(logrus.Level(logLevel))
	}

	logFD, err := strconv.Atoi(os.Getenv("_LIBCONTAINER_LOGPIPE"))
	if err != nil {
		return fmt.Errorf("unable to convert _LIBCONTAINER_LOGPIPE: %w", err)
	}

	logrus.SetOutput(os.NewFile(uintptr(logFD), "logpipe"))
	logrus.SetFormatter(new(logrus.JSONFormatter))
	logrus.Debug("child process in init()")

	// Only init processes have FIFOFD.
	fifofd := -1
	envInitType := os.Getenv("_LIBCONTAINER_INITTYPE")
	it := initType(envInitType)
	if it == initStandard {
		envFifoFd := os.Getenv("_LIBCONTAINER_FIFOFD")
		if fifofd, err = strconv.Atoi(envFifoFd); err != nil {
			return fmt.Errorf("unable to convert _LIBCONTAINER_FIFOFD: %w", err)
		}
	}

	var consoleSocket *os.File
	if envConsole := os.Getenv("_LIBCONTAINER_CONSOLE"); envConsole != "" {
		console, err := strconv.Atoi(envConsole)
		if err != nil {
			return fmt.Errorf("unable to convert _LIBCONTAINER_CONSOLE: %w", err)
		}
		consoleSocket = os.NewFile(uintptr(console), "console-socket")
		defer consoleSocket.Close()
	}

	var pidfdSocket *os.File
	if envSockFd := os.Getenv("_LIBCONTAINER_PIDFD_SOCK"); envSockFd != "" {
		sockFd, err := strconv.Atoi(envSockFd)
		if err != nil {
			return fmt.Errorf("unable to convert _LIBCONTAINER_PIDFD_SOCK: %w", err)
		}
		pidfdSocket = os.NewFile(uintptr(sockFd), "pidfd-socket")
		defer pidfdSocket.Close()
	}

	// Get mount files (O_PATH).
	mountSrcFds, err := parseFdsFromEnv("_LIBCONTAINER_MOUNT_FDS")
	if err != nil {
		return err
	}

	// Get idmap fds.
	idmapFds, err := parseFdsFromEnv("_LIBCONTAINER_IDMAP_FDS")
	if err != nil {
		return err
	}

	// Get runc-dmz fds.
	var dmzExe *os.File
	if dmzFdStr := os.Getenv("_LIBCONTAINER_DMZEXEFD"); dmzFdStr != "" {
		dmzFd, err := strconv.Atoi(dmzFdStr)
		if err != nil {
			return fmt.Errorf("unable to convert _LIBCONTAINER_DMZEXEFD: %w", err)
		}
		unix.CloseOnExec(dmzFd)
		dmzExe = os.NewFile(uintptr(dmzFd), "runc-dmz")
	}

	// clear the current process's environment to clean any libcontainer
	// specific env vars.
	os.Clearenv()

	defer func() {
		if err := recover(); err != nil {
			if err2, ok := err.(error); ok {
				retErr = fmt.Errorf("panic from initialization: %w, %s", err2, debug.Stack())
			} else {
				retErr = fmt.Errorf("panic from initialization: %v, %s", err, debug.Stack())
			}
		}
	}()

	var config initConfig
	if err := json.NewDecoder(initPipe).Decode(&config); err != nil {
		return err
	}

	// If init succeeds, it will not return, hence none of the defers will be called.
	return containerInit(it, &config, syncPipe, consoleSocket, pidfdSocket, fifofd, logFD, dmzExe, mountFds{sourceFds: mountSrcFds, idmapFds: idmapFds})
}

func containerInit(t initType, config *initConfig, pipe *syncSocket, consoleSocket, pidfdSocket *os.File, fifoFd, logFd int, dmzExe *os.File, mountFds mountFds) error {
	if err := populateProcessEnvironment(config.Env); err != nil {
		return err
	}

	switch t {
	case initSetns:
		// mount and idmap fds must be nil in this case. We don't mount while doing runc exec.
		if mountFds.sourceFds != nil || mountFds.idmapFds != nil {
			return errors.New("mount and idmap fds must be nil; can't mount from exec")
		}

		i := &linuxSetnsInit{
			pipe:          pipe,
			consoleSocket: consoleSocket,
			pidfdSocket:   pidfdSocket,
			config:        config,
			logFd:         logFd,
			dmzExe:        dmzExe,
		}
		return i.Init()
	case initStandard:
		i := &linuxStandardInit{
			pipe:          pipe,
			consoleSocket: consoleSocket,
			pidfdSocket:   pidfdSocket,
			parentPid:     unix.Getppid(),
			config:        config,
			fifoFd:        fifoFd,
			logFd:         logFd,
			dmzExe:        dmzExe,
			mountFds:      mountFds,
		}
		return i.Init()
	}
	return fmt.Errorf("unknown init type %q", t)
}

// populateProcessEnvironment loads the provided environment variables into the
// current processes's environment.
func populateProcessEnvironment(env []string) error {
	for _, pair := range env {
		p := strings.SplitN(pair, "=", 2)
		if len(p) < 2 {
			return errors.New("invalid environment variable: missing '='")
		}
		name, val := p[0], p[1]
		if name == "" {
			return errors.New("invalid environment variable: name cannot be empty")
		}
		if strings.IndexByte(name, 0) >= 0 {
			return fmt.Errorf("invalid environment variable %q: name contains nul byte (\\x00)", name)
		}
		if strings.IndexByte(val, 0) >= 0 {
			return fmt.Errorf("invalid environment variable %q: value contains nul byte (\\x00)", name)
		}
		if err := os.Setenv(name, val); err != nil {
			return err
		}
	}
	return nil
}

// finalizeNamespace drops the caps, sets the correct user
// and working dir, and closes any leaked file descriptors
// before executing the command inside the namespace
func finalizeNamespace(config *initConfig) error {
	// Ensure that all unwanted fds we may have accidentally
	// inherited are marked close-on-exec so they stay out of the
	// container
	if err := utils.CloseExecFrom(config.PassedFilesCount + 3); err != nil {
		return fmt.Errorf("error closing exec fds: %w", err)
	}

	// we only do chdir if it's specified
	doChdir := config.Cwd != ""
	if doChdir {
		// First, attempt the chdir before setting up the user.
		// This could allow us to access a directory that the user running runc can access
		// but the container user cannot.
		err := unix.Chdir(config.Cwd)
		switch {
		case err == nil:
			doChdir = false
		case os.IsPermission(err):
			// If we hit an EPERM, we should attempt again after setting up user.
			// This will allow us to successfully chdir if the container user has access
			// to the directory, but the user running runc does not.
			// This is useful in cases where the cwd is also a volume that's been chowned to the container user.
		default:
			return fmt.Errorf("chdir to cwd (%q) set in config.json failed: %w", config.Cwd, err)
		}
	}

	caps := &configs.Capabilities{}
	if config.Capabilities != nil {
		caps = config.Capabilities
	} else if config.Config.Capabilities != nil {
		caps = config.Config.Capabilities
	}
	w, err := capabilities.New(caps)
	if err != nil {
		return err
	}
	// drop capabilities in bounding set before changing user
	if err := w.ApplyBoundingSet(); err != nil {
		return fmt.Errorf("unable to apply bounding set: %w", err)
	}
	// preserve existing capabilities while we change users
	if err := system.SetKeepCaps(); err != nil {
		return fmt.Errorf("unable to set keep caps: %w", err)
	}
	if err := setupUser(config); err != nil {
		return fmt.Errorf("unable to setup user: %w", err)
	}
	// Change working directory AFTER the user has been set up, if we haven't done it yet.
	if doChdir {
		if err := unix.Chdir(config.Cwd); err != nil {
			return fmt.Errorf("chdir to cwd (%q) set in config.json failed: %w", config.Cwd, err)
		}
	}
	if err := system.ClearKeepCaps(); err != nil {
		return fmt.Errorf("unable to clear keep caps: %w", err)
	}
	if err := w.ApplyCaps(); err != nil {
		return fmt.Errorf("unable to apply caps: %w", err)
	}
	return nil
}

// setupConsole sets up the console from inside the container, and sends the
// master pty fd to the config.Pipe (using cmsg). This is done to ensure that
// consoles are scoped to a container properly (see runc#814 and the many
// issues related to that). This has to be run *after* we've pivoted to the new
// rootfs (and the users' configuration is entirely set up).
func setupConsole(socket *os.File, config *initConfig, mount bool) error {
	defer socket.Close()
	// At this point, /dev/ptmx points to something that we would expect. We
	// used to change the owner of the slave path, but since the /dev/pts mount
	// can have gid=X set (at the users' option). So touching the owner of the
	// slave PTY is not necessary, as the kernel will handle that for us. Note
	// however, that setupUser (specifically fixStdioPermissions) *will* change
	// the UID owner of the console to be the user the process will run as (so
	// they can actually control their console).

	pty, slavePath, err := console.NewPty()
	if err != nil {
		return err
	}
	// After we return from here, we don't need the console anymore.
	defer pty.Close()

	if config.ConsoleHeight != 0 && config.ConsoleWidth != 0 {
		err = pty.Resize(console.WinSize{
			Height: config.ConsoleHeight,
			Width:  config.ConsoleWidth,
		})

		if err != nil {
			return err
		}
	}

	// Mount the console inside our rootfs.
	if mount {
		if err := mountConsole(slavePath); err != nil {
			return err
		}
	}
	// While we can access console.master, using the API is a good idea.
	if err := utils.SendRawFd(socket, pty.Name(), pty.Fd()); err != nil {
		return err
	}
	runtime.KeepAlive(pty)

	// Now, dup over all the things.
	return dupStdio(slavePath)
}

// syncParentReady sends to the given pipe a JSON payload which indicates that
// the init is ready to Exec the child process. It then waits for the parent to
// indicate that it is cleared to Exec.
func syncParentReady(pipe *syncSocket) error {
	// Tell parent.
	if err := writeSync(pipe, procReady); err != nil {
		return err
	}
	// Wait for parent to give the all-clear.
	return readSync(pipe, procRun)
}

// syncParentHooks sends to the given pipe a JSON payload which indicates that
// the parent should execute pre-start hooks. It then waits for the parent to
// indicate that it is cleared to resume.
func syncParentHooks(pipe *syncSocket) error {
	// Tell parent.
	if err := writeSync(pipe, procHooks); err != nil {
		return err
	}
	// Wait for parent to give the all-clear.
	return readSync(pipe, procHooksDone)
}

// syncParentSeccomp sends the fd associated with the seccomp file descriptor
// to the parent, and wait for the parent to do pidfd_getfd() to grab a copy.
func syncParentSeccomp(pipe *syncSocket, seccompFd *os.File) error {
	if seccompFd == nil {
		return nil
	}
	defer seccompFd.Close()

	// Tell parent to grab our fd.
	//
	// Notably, we do not use writeSyncFile here because a container might have
	// an SCMP_ACT_NOTIFY action on sendmsg(2) so we need to use the smallest
	// possible number of system calls here because all of those syscalls
	// cannot be used with SCMP_ACT_NOTIFY as a result (any syscall we use here
	// before the parent gets the file descriptor would deadlock "runc init" if
	// we allowed it for SCMP_ACT_NOTIFY). See seccomp.InitSeccomp() for more
	// details.
	if err := writeSyncArg(pipe, procSeccomp, seccompFd.Fd()); err != nil {
		return err
	}
	// Wait for parent to tell us they've grabbed the seccompfd.
	return readSync(pipe, procSeccompDone)
}

// setupUser changes the groups, gid, and uid for the user inside the container
func setupUser(config *initConfig) error {
	// Set up defaults.
	defaultExecUser := user.ExecUser{
		Uid:  0,
		Gid:  0,
		Home: "/",
	}

	passwdPath, err := user.GetPasswdPath()
	if err != nil {
		return err
	}

	groupPath, err := user.GetGroupPath()
	if err != nil {
		return err
	}

	execUser, err := user.GetExecUserPath(config.User, &defaultExecUser, passwdPath, groupPath)
	if err != nil {
		return err
	}

	var addGroups []int
	if len(config.AdditionalGroups) > 0 {
		addGroups, err = user.GetAdditionalGroupsPath(config.AdditionalGroups, groupPath)
		if err != nil {
			return err
		}
	}

	// Rather than just erroring out later in setuid(2) and setgid(2), check
	// that the user is mapped here.
	if _, err := config.Config.HostUID(execUser.Uid); err != nil {
		return errors.New("cannot set uid to unmapped user in user namespace")
	}
	if _, err := config.Config.HostGID(execUser.Gid); err != nil {
		return errors.New("cannot set gid to unmapped user in user namespace")
	}

	if config.RootlessEUID {
		// We cannot set any additional groups in a rootless container and thus
		// we bail if the user asked us to do so. TODO: We currently can't do
		// this check earlier, but if libcontainer.Process.User was typesafe
		// this might work.
		if len(addGroups) > 0 {
			return errors.New("cannot set any additional groups in a rootless container")
		}
	}

	// Before we change to the container's user make sure that the processes
	// STDIO is correctly owned by the user that we are switching to.
	if err := fixStdioPermissions(execUser); err != nil {
		return err
	}

	setgroups, err := os.ReadFile("/proc/self/setgroups")
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// This isn't allowed in an unprivileged user namespace since Linux 3.19.
	// There's nothing we can do about /etc/group entries, so we silently
	// ignore setting groups here (since the user didn't explicitly ask us to
	// set the group).
	allowSupGroups := !config.RootlessEUID && string(bytes.TrimSpace(setgroups)) != "deny"

	if allowSupGroups {
		suppGroups := append(execUser.Sgids, addGroups...)
		if err := unix.Setgroups(suppGroups); err != nil {
			return &os.SyscallError{Syscall: "setgroups", Err: err}
		}
	}

	if err := unix.Setgid(execUser.Gid); err != nil {
		return err
	}
	if err := unix.Setuid(execUser.Uid); err != nil {
		return err
	}

	// if we didn't get HOME already, set it based on the user's HOME
	if envHome := os.Getenv("HOME"); envHome == "" {
		if err := os.Setenv("HOME", execUser.Home); err != nil {
			return err
		}
	}
	return nil
}

// fixStdioPermissions fixes the permissions of PID 1's STDIO within the container to the specified user.
// The ownership needs to match because it is created outside of the container and needs to be
// localized.
func fixStdioPermissions(u *user.ExecUser) error {
	var null unix.Stat_t
	if err := unix.Stat("/dev/null", &null); err != nil {
		return &os.PathError{Op: "stat", Path: "/dev/null", Err: err}
	}
	for _, file := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		var s unix.Stat_t
		if err := unix.Fstat(int(file.Fd()), &s); err != nil {
			return &os.PathError{Op: "fstat", Path: file.Name(), Err: err}
		}

		// Skip chown if uid is already the one we want or any of the STDIO descriptors
		// were redirected to /dev/null.
		if int(s.Uid) == u.Uid || s.Rdev == null.Rdev {
			continue
		}

		// We only change the uid (as it is possible for the mount to
		// prefer a different gid, and there's no reason for us to change it).
		// The reason why we don't just leave the default uid=X mount setup is
		// that users expect to be able to actually use their console. Without
		// this code, you couldn't effectively run as a non-root user inside a
		// container and also have a console set up.
		if err := file.Chown(u.Uid, int(s.Gid)); err != nil {
			// If we've hit an EINVAL then s.Gid isn't mapped in the user
			// namespace. If we've hit an EPERM then the inode's current owner
			// is not mapped in our user namespace (in particular,
			// privileged_wrt_inode_uidgid() has failed). Read-only
			// /dev can result in EROFS error. In any case, it's
			// better for us to just not touch the stdio rather
			// than bail at this point.

			if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EPERM) || errors.Is(err, unix.EROFS) {
				continue
			}
			return err
		}
	}
	return nil
}

// setupNetwork sets up and initializes any network interface inside the container.
func setupNetwork(config *initConfig) error {
	for _, config := range config.Networks {
		strategy, err := getStrategy(config.Type)
		if err != nil {
			return err
		}
		if err := strategy.initialize(config); err != nil {
			return err
		}
	}
	return nil
}

func setupRoute(config *configs.Config) error {
	for _, config := range config.Routes {
		_, dst, err := net.ParseCIDR(config.Destination)
		if err != nil {
			return err
		}
		src := net.ParseIP(config.Source)
		if src == nil {
			return fmt.Errorf("Invalid source for route: %s", config.Source)
		}
		gw := net.ParseIP(config.Gateway)
		if gw == nil {
			return fmt.Errorf("Invalid gateway for route: %s", config.Gateway)
		}
		l, err := netlink.LinkByName(config.InterfaceName)
		if err != nil {
			return err
		}
		route := &netlink.Route{
			Scope:     netlink.SCOPE_UNIVERSE,
			Dst:       dst,
			Src:       src,
			Gw:        gw,
			LinkIndex: l.Attrs().Index,
		}
		if err := netlink.RouteAdd(route); err != nil {
			return err
		}
	}
	return nil
}

func setupRlimits(limits []configs.Rlimit, pid int) error {
	for _, rlimit := range limits {
		if err := unix.Prlimit(pid, rlimit.Type, &unix.Rlimit{Max: rlimit.Hard, Cur: rlimit.Soft}, nil); err != nil {
			return fmt.Errorf("error setting rlimit type %v: %w", rlimit.Type, err)
		}
	}
	return nil
}

func setupScheduler(config *configs.Config) error {
	attr, err := configs.ToSchedAttr(config.Scheduler)
	if err != nil {
		return err
	}
	if err := unix.SchedSetAttr(0, attr, 0); err != nil {
		if errors.Is(err, unix.EPERM) && config.Cgroups.CpusetCpus != "" {
			return errors.New("process scheduler can't be used together with AllowedCPUs")
		}
		return fmt.Errorf("error setting scheduler: %w", err)
	}
	return nil
}

func setupPersonality(config *configs.Config) error {
	return system.SetLinuxPersonality(config.Personality.Domain)
}

// signalAllProcesses freezes then iterates over all the processes inside the
// manager's cgroups sending the signal s to them.
func signalAllProcesses(m cgroups.Manager, s unix.Signal) error {
	if !m.Exists() {
		return ErrNotRunning
	}
	// Use cgroup.kill, if available.
	if s == unix.SIGKILL {
		if p := m.Path(""); p != "" { // Either cgroup v2 or hybrid.
			err := cgroups.WriteFile(p, "cgroup.kill", "1")
			if err == nil || !errors.Is(err, os.ErrNotExist) {
				return err
			}
			// Fallback to old implementation.
		}
	}

	if err := m.Freeze(configs.Frozen); err != nil {
		logrus.Warn(err)
	}
	pids, err := m.GetAllPids()
	if err != nil {
		if err := m.Freeze(configs.Thawed); err != nil {
			logrus.Warn(err)
		}
		return err
	}
	for _, pid := range pids {
		err := unix.Kill(pid, s)
		if err != nil && err != unix.ESRCH {
			logrus.Warnf("kill %d: %v", pid, err)
		}
	}
	if err := m.Freeze(configs.Thawed); err != nil {
		logrus.Warn(err)
	}

	return nil
}

// setupPidfd opens a process file descriptor of init process, and sends the
// file descriptor back to the socket.
func setupPidfd(socket *os.File, initType string) error {
	defer socket.Close()

	pidFd, err := unix.PidfdOpen(os.Getpid(), 0)
	if err != nil {
		return fmt.Errorf("failed to pidfd_open: %w", err)
	}

	if err := utils.SendRawFd(socket, initType, uintptr(pidFd)); err != nil {
		unix.Close(pidFd)
		return fmt.Errorf("failed to send pidfd on socket: %w", err)
	}
	return unix.Close(pidFd)
}
