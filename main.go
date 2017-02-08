package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Data struct for JIRA JSON parsing.
type Data struct {
	WebhookEvent string
	User         struct {
		Name        string
		AvatarUrls  map[string]string
		DisplayName string
	}
	Issue struct {
		Self   string
		Key    string
		Fields struct {
			Issuetype struct {
				IconURL string
				Name    string
			}
			Summary string
		}
	}
	Comment struct {
		Body string
	}
	Changelog struct {
		Items []struct {
			Field      string
			FromString string
			ToString   string
		}
	}
}

// Message structure for Mattermost JSON creation.
type Message struct {
	Pretext  string `json:"pretext,omitempty"`
	Text     string `json:"text"`
	Username string `json:"username"`
	IconURL  string `json:"icon_url"`
	Color    string `json:"color,omitempty"`
}

func getMessage(request *http.Request) []byte {
	//initialisation
	var JiraMultilineFields = map[string]bool{
		// move field list to configuration file?
		"Acceptance Criteria": true,
		"Demo Script":         true,
		"Release Notes Text":  true,
		"Description":         true,
		"Deployment Notes":    true,
	}

	//replacer for Jira emoji
	wikiReplacer := strings.NewReplacer(
		":)", ":simple_smile:",
		":(", ":worried:",
		" :P", ":stuck_out_tongue_winking_eye:", //added space, xml with namespace problem
		" :D", ":grinning:", //xml namespace
		";)", ":wink:",
		"(y)", ":thumbsup:",
		"(n)", ":thumbsdown:",
		"(i)", ":information_source:",
		"(/)", ":white_check_mark:",
		"(x)", ":x:",
		"(!)", ":warning:",
		"(-)", ":no_entry:",
		"(?)", ":question:",
		"(on)", ":bulb:",
		"(*)", ":star:",
		"----", "---",
		"{code}", "```",
		"{code:xml}", "```xml ",
		"{code:java}", "```java ",
		"{code:javascript}", "```javascript ",
		"{code:sql}", "```sql ",
		"# ", "1. ",
		"## ", "  1. ",
		"### ", "    1. ",
		"** ", "  * ",
		"*** ", "    * ",
		"-- ", "  * ",
		"--- ", "    * ",
		"h1.", "#",
		"h2.", "##",
		"h3.", "###",
		"h4.", "####",
		"h5.", "#####",
		"h6.", "######",
	)

	// Parse JSON from JIRA
	decoder := json.NewDecoder(request.Body)
	var data Data
	decoder.Decode(&data)

	// Get JIRA URL from "issue" section in JSON
	u, _ := url.Parse(data.Issue.Self)

	// Select action
	var action, comment string
	switch data.WebhookEvent {
	case "jira:issue_created":
		action = "created"
	case "jira:issue_updated":
		action = "updated"
	case "jira:issue_deleted":
		action = "deleted"
	}

	//Process new comment
	if len(data.Comment.Body) > 0 {
		comment = fmt.Sprintf("\nComment:\n%s\n", wikiReplacer.Replace(data.Comment.Body))
	}

	// Process changelog
	var changelog string
	var color string
	if len(data.Changelog.Items) > 0 {
		for _, item := range data.Changelog.Items {
			itemName := strings.ToUpper(string(item.Field[0])) + item.Field[1:]
			if item.FromString == "" {
				item.FromString = "None"
			}
			if JiraMultilineFields[itemName] {
				changelog += fmt.Sprintf(
					"\nChanged **%s**:\n\n---\n%s\n",
					itemName,
					wikiReplacer.Replace(item.ToString),
				)
			} else {
				changelog += fmt.Sprintf(
					"\n%s: ~~%s~~ %s",
					itemName,
					strings.Trim(item.FromString, " "),
					item.ToString,
				)
			}
		}
	}
	if strings.HasPrefix(data.Issue.Key, "SVD") {
		color = "ff0000"
	}

	// Create message for Mattermost
	text := fmt.Sprintf(
		//Message format:
		//![user_icon](user_icon_link)[UserFirstName UserSecondName](user_link) commented task ![task_icon](task_icon link)[TSK-42](issue_link) "Test task"
		//Status: ~~Done~~ Finished
		//>Comment text
		"![user_icon](%s) [%s](%s://%s/secure/ViewProfile.jspa?name=%s) %s %s ![task_icon](%s) [%s](%s://%s/browse/%s) \"%s\"%s%s",
		data.User.AvatarUrls["16x16"],
		data.User.DisplayName,
		u.Scheme,
		u.Host,
		data.User.Name,
		action,
		strings.ToLower(data.Issue.Fields.Issuetype.Name),
		data.Issue.Fields.Issuetype.IconURL,
		data.Issue.Key,
		u.Scheme,
		u.Host,
		data.Issue.Key,
		data.Issue.Fields.Summary,
		changelog,
		comment,
	)
	println(text)

	message := Message{
		Color:    color,
		Text:     text,
		Username: "JIRA",
		IconURL:  "https://raw.githubusercontent.com/hhommersom/mattermost-jira/master/logo-02.png",
	}

	JSONMessage, _ := json.Marshal(message)

	println(JSONMessage)

	return JSONMessage
}

func index(_ http.ResponseWriter, r *http.Request) {

	println(".")
	// Get mattermost URL
	mattermostHookURL := r.URL.Query().Get("mattermost_hook_url")

	println(mattermostHookURL)

	if len(mattermostHookURL) > 0 {
		// Get message from JIRA JSON request
		message := getMessage(r)

		// Create http-client
		req, _ := http.NewRequest("POST", mattermostHookURL, bytes.NewBuffer(message))
		req.Header.Set("Content-Type", "application/json")

		// Send data to Mattermost
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()
	}
}

func main() {

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}
	http.HandleFunc("/", index)
	http.ListenAndServe(":"+port, nil)
}
