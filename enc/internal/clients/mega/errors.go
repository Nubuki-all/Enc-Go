package mega

import (
	"errors"
	"fmt"
)

var (
	// General errors
	EINTERNAL  = errors.New("internal error occured")
	EARGS      = errors.New("invalid arguments")
	EAGAIN     = errors.New("try again")
	ERATELIMIT = errors.New("rate limit reached")
	EBADRESP   = errors.New("bad response from server")

	// Upload errors
	EFAILED  = errors.New("the upload failed. Please restart it from scratch")
	ETOOMANY = errors.New("too many concurrent IP addresses are accessing this upload target URL")
	ERANGE   = errors.New("the upload file packet is out of range or not starting and ending on a chunk boundary")
	EEXPIRED = errors.New("the upload target URL you are trying to access has expired. Please request a fresh one")

	// Filesystem/Account errors
	ENOENT              = errors.New("object (typically, node or user) not found")
	ECIRCULAR           = errors.New("circular linkage attempted")
	EACCESS             = errors.New("access violation")
	EEXIST              = errors.New("trying to create an object that already exists")
	EINCOMPLETE         = errors.New("trying to access an incomplete resource")
	EKEY                = errors.New("a decryption operation failed")
	ESID                = errors.New("invalid or expired user session, please relogin")
	EBLOCKED            = errors.New("user blocked")
	EOVERQUOTA          = errors.New("request over quota")
	ETEMPUNAVAIL        = errors.New("resource temporarily not available, please try again later")
	EMACMISMATCH        = errors.New("mac verification failed")
	EBADATTR            = errors.New("bad node attribute")
	ETOOMANYCONNECTIONS = errors.New("too many connections on this resource.")
	EWRITE              = errors.New("file could not be written to (or failed post-write integrity check).")
	EREAD               = errors.New("file could not be read from (or changed unexpectedly during reading).")
	EAPPKEY             = errors.New("invalid or missing application key.")
	ESSL                = errors.New("ssl verification failed")
	EGOINGOVERQUOTA     = errors.New("not enough quota")
	EMFAREQUIRED        = errors.New("multi-factor authentication required")

	// Config errors
	EWORKER_LIMIT_EXCEEDED = errors.New("maximum worker limit exceeded")

	EUNSUPPORTEDLINK = errors.New("unsupported mega url format")
)

type ErrorMsg int

func parseError(errno ErrorMsg) error {
	switch {
	case errno == 0:
		return nil
	case errno == -1:
		return EINTERNAL
	case errno == -2:
		return EARGS
	case errno == -3:
		return EAGAIN
	case errno == -4:
		return ERATELIMIT
	case errno == -5:
		return EFAILED
	case errno == -6:
		return ETOOMANY
	case errno == -7:
		return ERANGE
	case errno == -8:
		return EEXPIRED
	case errno == -9:
		return ENOENT
	case errno == -10:
		return ECIRCULAR
	case errno == -11:
		return EACCESS
	case errno == -12:
		return EEXIST
	case errno == -13:
		return EINCOMPLETE
	case errno == -14:
		return EKEY
	case errno == -15:
		return ESID
	case errno == -16:
		return EBLOCKED
	case errno == -17:
		return EOVERQUOTA
	case errno == -18:
		return ETEMPUNAVAIL
	case errno == -19:
		return ETOOMANYCONNECTIONS
	case errno == -20:
		return EWRITE
	case errno == -21:
		return EREAD
	case errno == -22:
		return EAPPKEY
	case errno == -23:
		return ESSL
	case errno == -24:
		return EGOINGOVERQUOTA
	case errno == -26:
		return EMFAREQUIRED
	}

	return fmt.Errorf("unknown mega error %d", errno)
}
