# lago - CLI for AWS Lambda and the Lambda Go Runtime
This utility allows uploading and downloading existing Lambda functions and deploying Go source and support files to Lambda functions configured for the Go runtime.

## Install

```go get github.com/cloudinterfaces/lago``` and put the resulting binary somewhere in your PATH.

## Usage
```
get - download a Lambda function to a directory
put - upload the contents of a directory to a Lambda function
deploy - compile and run Go source and upload to a Lambda function
```
See help (-? flag) until it says to refer back to this.

## Static files
```lago deploy``` allows static files to be uploaded with the compiled handler. They are specified on the commandline as:
```{[base(":"|";")]path}```

Base is a Unix path relative to the root of the underlying Lambda filesystem. The initial ```/``` may be omitted.

If path is a regular file, it is uploaded to base/Base(path) of the function environment. For example on unix-like systems:

```static:/etc/passwd``` would result in the contents of /etc/passwd in a file at ```/static/passwd``` in the Lambda environment. ```/etc/passwd``` would result in the contents of /etc/passwd in a file at ```/passwd``` in the Lambda environment.

If path is a directory, the contents are uploaded to base. If path ends in a path separator, the contents are uploaded recursively, with subdirectory names joined to base. For example given the directory structure:
```
/opt
/opt/file1.txt
/opt/file2.txt
/opt/html/template.tmpl
```

If ```static:/opt``` is specified, the Lambda filesystem will contain:

```
/static
/static/file1.txt
/static/file2.txt
```

If ```opt:/opt/``` (/opt already exists in the Lambda function filesystem) is specified, the Lambda filesystem will contain:

```
/opt
/opt/file1.txt
/opt/file2.txt
/opt/html
/opt/html/template.tmpl
```

Note the maximum upload size of a Lambda function is 50 megabytes and themaximum unpacked size must be less than 250 megabytes. Note also thoughtless uploading of files may damage the Lambda runtime environment, which is based on Amazon Linux (for example ```etc:/etc/resolv.conf``` is probably a very bad idea).

## Related
The [lh](https://github.com/cloudinterfaces/lh) package makes it easy to deploy (many or most) http.Handlers with the AWS Lambda Go runtime.
