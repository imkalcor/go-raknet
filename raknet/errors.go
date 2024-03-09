package raknet

import "errors"

var DPL_ERROR = errors.New("the client has tried to login again")

var IFD_ERROR = errors.New("the buffer does not appear to have flag datagram")

var IRT_ERROR = errors.New("an exception has occured while trying to parse the receipt record type")

var ILN_ERROR = errors.New("an exception has occured while trying to parse the length (=0)")

var MFC_ERROR = errors.New("the datagram exceeds the maximum number of frames it is allowed to contain")

var EMF_ERROR = errors.New("the datagram exceeds the maximum number of fragments it is allowed to have")
