package sample

type SampleObject struct {
	Id          string
	Name        string
	Age         int64
	IsMale      bool
	AnotherUser SampleObject2
}

type SampleObject2 struct {
	Id     string
	Name   string
	Age    int64
	IsMale bool
}
