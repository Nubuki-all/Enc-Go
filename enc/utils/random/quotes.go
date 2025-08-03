package random

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

// zenQuote models the JSON object returned by ZenQuotes
type zenQuote struct {
    Q string `json:"q"` // quote text
    A string `json:"a"` // author
}

// EnQuotes fetches a random quote from ZenQuotes 
func EnQuotes() string {
    em := Enmoji()
    client := http.Client{Timeout: 5 * time.Second}

    resp, err := client.Get("https://zenquotes.io/api/random")
    if err != nil {
        return fmt.Sprintf("%s **Nubuki-all:** `Could not fetch quote: %v`", em, err)
    }
    defer resp.Body.Close()

    var arr []zenQuote
    if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
        return fmt.Sprintf("%s **Nubuki-all:** `Invalid quote response`", em)
    }
    if len(arr) == 0 {
        return fmt.Sprintf("%s **Nubuki-all:** `No quote received`", em)
    }

    q := arr[0]
    text := q.Q
    // truncate if longer than 2046 chars
    if len(text) > 2046 {
        text = text[:2045] + "…"
    }
    return fmt.Sprintf("%s **%s:** `%s`", em, q.A, text)
}
