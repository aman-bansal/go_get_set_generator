go_get_set_generator
====================
This is an automatic code generator of Get and Set functions for go structs.

Installation
------------

You need to install the go_get_set_generator. To get the latest released version use:

```GO111MODULE=on go get github.com/aman-bansal/go_get_set_generator@latest```

Running go_get_set_generator
----------------------------
```get_set_generate -source=path/fileName.go```

this will generate a new file with name `fileName_getter_setter.go` containing all the 
get set functions for the structs present in the specified file. 


Scope Of Improvement 
--------------------
1. Add command line option to get the list of structs
2. Add command line option to set the flag to get either `Get` functions or `Set` functions only

Issue Reporting
----------------
If you found any issues or wanted to suggest any enhancement, do create an issue in the repo itself or shoot an email to bansalaman2905[at]gmail[dot]com.
