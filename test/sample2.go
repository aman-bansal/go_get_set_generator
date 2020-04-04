package test

import "github.com/aman-bansal/go_get_set_generator/test/sample"

type SampleObject3 struct {
	Id          string
	Name        string
	Age         int64
	IsMale      bool
	AnotherUser []sample.SampleObject2
}

type SampleObject4 struct {
	Id     string
	Name   string
	IsMale sample.SampleObject
	Age    SampleObject3
}
