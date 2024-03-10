package raknet

import "errors"

// This error is sent when a client tries to login to the server again
var DPL_ERROR = errors.New("the client has tried to login again")

// This error is sent when a buffer does not start with the flag datagram as all
// raknet datagrams must have the datagram flag.
var IFD_ERROR = errors.New("the buffer does not appear to have flag datagram")

// This error is sent when the raknet receipt record has an invalid record type value encoded.
var IRT_ERROR = errors.New("an exception has occured while trying to parse the receipt record type")

// This error is sent when a raknet message has invalid length encoded <= 0
var ILN_ERROR = errors.New("an exception has occured while trying to parse the length (=0)")

// This error is sent when the datagram exceeds the maximum number of frames it is allowed to contain
var MFC_ERROR = errors.New("the datagram exceeds the maximum number of frames it is allowed to contain")

// This error is sent when the datagram exceeds the maximum number of fragments it is allowed to have
var EMF_ERROR = errors.New("the datagram exceeds the maximum number of fragments it is allowed to have")
