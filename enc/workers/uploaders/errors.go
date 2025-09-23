package uploaders

const ErrRetry UploadError = "try again"

type UploadError string

func (e UploadError) Error() string {
	return string(e)
}
