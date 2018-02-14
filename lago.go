package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/cloudinterfaces/lago/filesystem"
)

func init() {
	log.SetFlags(0)
	log.SetPrefix("lago: ")
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func usage(s string, f func()) func() {
	log.SetPrefix("")
	return func() {
		log.Println(strings.TrimSpace(s))
		f()
	}
}

func defaultregion() string {
	const defaultregion = "us-east-1"
	if s := os.Getenv("AWS_REGION"); len(s) > 0 {
		return s
	}
	return defaultregion
}

// Debug adds logging flags.
var Debug = flag.Bool("debug", false, "Verbose error log")

// Lambda is a shared Lambda API instance.
var svc *lambda.Lambda

// Region is the AWS region.
var Region = flag.String("region", defaultregion(), "AWS region, overridden by environment AWS_REGION")

func list() {
	input := &lambda.ListFunctionsInput{}
	for {
		res, err := svc.ListFunctions(input)
		if err != nil {
			log.Fatal(err)
		}
		for _, f := range res.Functions {
			if *f.Runtime == `go1.x` {
				fmt.Println(*f.FunctionName)
			}
		}
		if res.NextMarker == nil {
			break
		}
		input.Marker = res.NextMarker
	}
}

func versions(args []string) {
	print := fmt.Printf
	fs := flag.NewFlagSet("versions", flag.ExitOnError)
	long := fs.Bool("l", false, "Long output")
	check(fs.Parse(args))
	fn := fs.Arg(0)
	if len(fn) == 0 {
		log.Fatal("Function name required")
	}
	req := &lambda.ListVersionsByFunctionInput{
		FunctionName: &fn,
	}
	if *long {
		os.Stdout.Write([]byte{'\n'})
		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 16, 8, 0, '\t', 0)
		print = func(f string, i ...interface{}) (int, error) {
			return fmt.Fprintf(w, f, i...)
		}
		print("Version\tModified\tSHA256[:8]\n")
		defer w.Flush()
	}
	for {
		res, err := svc.ListVersionsByFunction(req)
		check(err)
		for _, c := range res.Versions {
			switch *long {
			case true:
				buf, _ := base64.StdEncoding.DecodeString(*c.CodeSha256)
				print("%s\t%s\t%x\n", *c.Version, *c.LastModified, buf[:8])
			default:
				print("%s\n", *c.Version)
			}
		}
		if req.Marker = res.NextMarker; req.Marker == nil {
			break
		}
	}
}

func get(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	force := fs.Bool("f", false, "Force purge of destination")
	version := fs.String("ver", "", "Version or alias")
	fs.Usage = usage(`Usage of lago get:
	lago [flags] get [-f] funcname destination_directory
	
	If -f is not true, interactive prompt if destination_directory
	is not empty.

	Flags:
	`, fs.PrintDefaults)
	err := fs.Parse(args)
	check(err)
	fn := fs.Arg(0)
	if len(fn) == 0 {
		log.Fatal("Function name required")
	}
	odir := fs.Arg(1)
	if len(odir) == 0 {
		log.Fatal("Output directory required")
	}
	f, err := os.Open(odir)
	check(err)
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil && err != io.EOF {
		log.Fatal(err)
	}
	if len(names) > 0 {
		if !*force {
			fmt.Fprintf(os.Stderr, "Directory %s is not empty, purge? [y/N]\n", odir)
			scanner := bufio.NewScanner(os.Stdin)
			if !scanner.Scan() {
				return
			}
			switch scanner.Text() {
			case "Y", "y":
			default:
				fmt.Fprintln(os.Stderr, `Aborting`)
				os.Exit(1)
			}
		}
		for _, name := range names {
			err = os.RemoveAll(filepath.Join(odir, name))
			check(err)
		}
	}
	input := &lambda.GetFunctionInput{FunctionName: &fn}
	if len(*version) > 0 {
		input.Qualifier = version
	}
	res, err := svc.GetFunction(input)
	check(err)
	tf, err := ioutil.TempFile("", "")
	check(err)
	defer os.Remove(tf.Name())
	defer tf.Close()
	get, err := http.Get(*res.Code.Location)
	check(err)
	defer get.Body.Close()
	_, err = io.Copy(tf, get.Body)
	check(err)
	_, err = tf.Seek(0, 0)
	check(err)
	st, err := tf.Stat()
	check(err)
	r, err := zip.NewReader(tf, st.Size())
	check(err)
	for _, f := range r.File {
		dir, _ := path.Split(f.Name)
		if len(dir) > 0 {
			check(os.MkdirAll(filepath.Join(odir, filepath.FromSlash(dir)), 0755))
		}
		of, err := os.Create(filepath.Join(odir, filepath.FromSlash(f.Name)))
		check(err)
		zr, err := f.Open()
		check(err)
		_, err = io.Copy(of, zr)
		check(err)
		of.Close()
		zr.Close()
	}
}

func put(args []string) {
	fs := flag.NewFlagSet("put", flag.ExitOnError)
	fs.Usage = usage(`Usage for lago put:
	lago [flags] put funcname directory

	All files in directory will be uploaded recursively to the function funcname.
		`, fs.PrintDefaults)
	version := fs.Bool("ver", false, "Create new version")
	err := fs.Parse(args)
	if err != nil {
		log.Fatal(err)
	}
	fn := fs.Arg(0)
	if len(fn) == 0 {
		log.Fatal("Function required")
	}
	idir := fs.Arg(1)
	fi, err := os.Stat(idir)
	if err != nil {
		log.Fatal(err)
	}
	if !fi.IsDir() {
		log.Fatalf("Input %s is not a directory", idir)
	}
	tf := new(bytes.Buffer)
	zw := zip.NewWriter(tf)
	if err = filesystem.ZipWalk(zw, idir, "", true); err != nil {
		log.Fatal(err)
	}
	if err = zw.Close(); err != nil {
		log.Fatal(err)
	}
	params := &lambda.UpdateFunctionCodeInput{
		FunctionName: &fn,
		ZipFile:      tf.Bytes(),
	}
	ufc, err := svc.UpdateFunctionCode(params)
	check(err)
	if *version {
		pv := &lambda.PublishVersionInput{
			FunctionName: &fn,
			RevisionId:   ufc.RevisionId,
		}
		_, err = svc.PublishVersion(pv)
		check(err)
	}
}

func deploy(args []string) {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	fs.Usage = usage(`Usage of lago deploy:
lago [flags] deploy -func funcname -target buildtarget [-all] {[base(;:)]path}

The optional {[base:]path} arguments add static files to the Lambda function,
which is useful for template files, executables, or even storing the source
of a function in Lambda. The base component specifies the path within
the Lambda environment. The path component specifies a file that exists in the
local filesystem. If path is a regular file, the Lambda environment will contain
base/filename. If base is not specified or empty, filename will exist in the
root of the Lambda environment. If path is a directory, the contents are added
recursively if a trailing separator exists, non-recursively otherwise
(see README.md).

Flags:
			`, fs.PrintDefaults)
	allfiles := fs.Bool("all", false, "Do not exclude source files if static files specified")
	Func := fs.String("func", "", "Lambda function name")
	Target := fs.String("target", "", "Build target (Go source file or main package directory)")
	version := fs.Bool("ver", false, "Create new version")
	err := fs.Parse(args)
	check(err)
	gobin, err := exec.LookPath("go")
	check(err)
	fn := *Func
	if len(fn) == 0 {
		log.Fatal("Flag -func missing")
	}
	target := *Target
	if len(target) == 0 {
		log.Fatal("Flag -target required")
	}
	var handlername string
	{
		input := lambda.GetFunctionConfigurationInput{
			FunctionName: &fn,
		}
		res, err := svc.GetFunctionConfiguration(&input)
		check(err)
		if r := *res.Runtime; r != `go1.x` {
			log.Fatalf("Runtime for %s is %s", fn, r)
		}
		handlername = *res.Handler
	}
	td, err := ioutil.TempDir("", "lmao-")
	check(err)
	switch *Debug {
	case true:
		log.Println("Preserving temporary directory:", td)
	default:
		defer os.RemoveAll(td)
	}
	execfile := filepath.Join(td, handlername)
	cmd := exec.Command(gobin, "build", "-o", execfile, target)
	if lt := os.Getenv("LAMBDA_TAGS"); len(lt) > 0 {
		cmd = exec.Command(gobin, "build", "-o", execfile, "-tags", lt, target)
	}
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	o, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(string(o))
		log.Fatal(err)
	}
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, err := os.Open(execfile)
	check(err)
	defer f.Close()
	fi, err := f.Stat()
	check(err)
	zh, err := zip.FileInfoHeader(fi)
	check(err)
	zh.SetMode(0555)
	w, err := zw.CreateHeader(zh)
	check(err)
	_, err = io.Copy(w, f)
	check(err)
	if args := fs.Args(); len(args) > 0 {
		sep := string(os.PathListSeparator)
		for _, a := range args {
			var base, filename string
			parts := strings.SplitN(a, sep, 2)
			filename = parts[0]
			if len(parts) == 2 {
				base, filename = parts[0], parts[1]
			}
			f := filesystem.Zip
			if strings.HasSuffix(filename, sep) {
				f = filesystem.ZipWalk
			}
			check(f(zw, filename, base, *allfiles))
		}
	}
	check(zw.Close())
	if *Debug {
		log.Println("Writing zipfile.zip to temporary directory")
		zipfile := filepath.Join(td, "zipfile.zip")
		f, err := os.Create(zipfile)
		if err != nil {
			log.Println("Couldn't create zipfile!")
			log.Fatal(err)
		}
		defer f.Close()
		if _, err = f.Write(buf.Bytes()); err != nil {
			log.Println("Couldn't copy file")
			log.Fatal(err)
		}
	}
	params := &lambda.UpdateFunctionCodeInput{
		FunctionName: &fn,
		ZipFile:      buf.Bytes(),
	}
	ufc, err := svc.UpdateFunctionCode(params)
	check(err)
	if *version {
		pv := &lambda.PublishVersionInput{
			FunctionName: &fn,
			RevisionId:   ufc.RevisionId,
		}
		_, err = svc.PublishVersion(pv)
		check(err)
	}
}

func main() {
	flag.Usage = usage(`
	Usage of lago:
	lago [flags] Command [cmdflags] [parameters]
	
	Command is one of:
		get put deploy

	Use:
		lago command -?
	for help with commands.

	Flags:
		`, flag.PrintDefaults)
	if !flag.Parsed() {
		flag.Parse()
	}
	var sess = session.New(&aws.Config{
		Region: Region,
	})
	svc = lambda.New(sess)
	switch flag.Arg(0) {
	case ``:
		log.Fatal("Command required, list versions get put or deploy")
	case `list`:
		list()
	case `versions`:
		versions(flag.Args()[1:])
	case `get`:
		get(flag.Args()[1:])
	case `put`:
		put(flag.Args()[1:])
	case `deploy`:
		deploy(flag.Args()[1:])
	default:
		log.Fatalf("Unsupported command %s", flag.Arg(0))
	}
}
