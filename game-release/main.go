package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/grsmv/goweek"
	"github.com/njuettner/alexa"
)

const (
	apiURL           = "API_URL"
	NintendoSwitchID = "130"
	PS4ID            = "48"
	XboxOneID        = "49"
	PCID             = "6"
)

type GameReleaser interface {
	Release() string
}

type Config struct {
	APIKey string
}

type IGDBResponse []struct {
	ID   int `json:"id"`
	Game struct {
		ID               int     `json:"id"`
		Name             string  `json:"name"`
		Slug             string  `json:"slug"`
		URL              string  `json:"url"`
		CreatedAt        int64   `json:"created_at"`
		UpdatedAt        int64   `json:"updated_at"`
		Summary          string  `json:"summary"`
		Popularity       float64 `json:"popularity"`
		VersionParent    int     `json:"version_parent"`
		VersionTitle     string  `json:"version_title"`
		Category         int     `json:"category"`
		FirstReleaseDate int64   `json:"first_release_date"`
		Platforms        []int   `json:"platforms"`
		ReleaseDates     []struct {
			Category int    `json:"category"`
			Platform int    `json:"platform"`
			Date     int64  `json:"date"`
			Human    string `json:"human"`
			Y        int    `json:"y"`
			M        int    `json:"m"`
		} `json:"release_dates"`
		Screenshots []struct {
			URL          string `json:"url"`
			CloudinaryID string `json:"cloudinary_id"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
		} `json:"screenshots"`
		Cover struct {
			URL          string `json:"url"`
			CloudinaryID string `json:"cloudinary_id"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
		} `json:"cover"`
	} `json:"game"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	Category  int    `json:"category"`
	Platform  int    `json:"platform"`
	Date      int64  `json:"date"`
	Y         int    `json:"y"`
	M         int    `json:"m"`
	Human     string `json:"human"`
	Region    int    `json:"region,omitempty"`
}

func (cfg *Config) Release(intent, console, start, end string) (*alexa.Response, error) {
	platformID := findPlatformID(console)
	v := composeValues(
		addValue("fields", "*"),
		addValue("filter[platform][eq]", platformID),
		addValue("filter[date][gte]", start),
		addValue("filter[date][lte]", end),
		addValue("order", "popularity:desc"),
		addValue("limit", "50"),
		addValue("scroll", "1"),
		addValue("expand", "game"),
	)

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/release_dates/", apiURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-key", cfg.APIKey)
	req.Header.Add("Accept", "application/json")
	req.URL.RawQuery = v.Values.Encode()

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	igdbResponses := []IGDBResponse{}
	igdbResponse := IGDBResponse{}
	err = json.NewDecoder(resp.Body).Decode(&igdbResponse)
	if err != nil {
		return nil, err
	}

	igdbResponses = append(igdbResponses, igdbResponse)

	xcount := resp.Header.Get("X-Count")
	pageCount, err := strconv.Atoi(xcount)
	if err != nil {
		return nil, err
	}
	xNextPage := resp.Header.Get("X-Next-Page")
	// if x-count is greater than limit 50, we need to iterate again and again
	if pageCount > 50 {
		for j := 50; j <= pageCount; j += 50 {
			igdbResponse = IGDBResponse{}
			v = composeValues(
				addValue("fields", "*"),
				addValue("expand", "game"),
			)
			req, err = http.NewRequest("GET", fmt.Sprintf("%s%s", apiURL, xNextPage), nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("user-key", cfg.APIKey)
			req.Header.Add("Accept", "application/json")
			req.URL.RawQuery = v.Values.Encode()
			resp, err = client.Do(req)
			if err != nil {
				return nil, err
			}
			err = json.NewDecoder(resp.Body).Decode(&igdbResponse)
			if err != nil {
				return nil, err
			}
			igdbResponses = append(igdbResponses, igdbResponse)
		}
	}
	gameNames := []string{}

	for _, response := range igdbResponses {
		for _, values := range response {
			// check if release date is 2018-Q*, if so drop it
			if strings.Contains(values.Human, "Q") {
				continue
			}
			// Region Codes
			// 1	Europe (EU)
			// 2	North America (NA)
			// 3	Australia (AU)
			// 4	New Zealand (NZ)
			// 5	Japan (JP)
			// 6	China (CH)
			// 7	Asia (AS)
			// 8	Worldwide
			if values.Region == 1 || values.Region == 8 {
				gameNames = append(gameNames, values.Game.Name)
			}
		}
	}
	gameNames = removeDuplicates(gameNames)
	games := strings.Join(gameNames, ", ")
	count := strings.Count(games, ",")
	games = strings.Replace(games, ", ", " und ", count)
	games = strings.Replace(games, " und ", ", ", count-1)

	var speechText, cardText string
	cardText = strings.Replace(games, ",", "\n", -1)
	switch {
	case intent == "ReleasePreviousWeek":
		if len(games) > 0 {
			speechText = fmt.Sprintf("folgende spiele sind die letzte woche für %s erschienen:\n %s", console, games)
		} else {
			speechText = fmt.Sprintf("ich konnte leider keine %s game releases für die letzte woche finden", console)
			cardText = "Leider keine Releases :("
		}
	case intent == "ReleaseThisWeek":
		if len(games) > 0 {
			speechText = fmt.Sprintf("folgende spiele kommen diese woche für %s:\n %s", console, games)
		} else {
			speechText = fmt.Sprintf("ich konnte leider keine %s game releases für diese woche finden", console)
			cardText = "Leider keine Releases :("
		}
	case intent == "ReleaseNextWeek":
		if len(games) > 0 {
			speechText = fmt.Sprintf("folgende spiele kommen nächste woche für %s: %s", console, games)
		} else {
			speechText = fmt.Sprintf("ich konnte leider keine %s game releases für nächste woche finden", console)
			cardText = "Leider keine Releases :("
		}
	}
	alexaResponse := &alexa.StandardResponse{
		OutputSpeechText: speechText,
		CardTitle:        fmt.Sprintf("%s - Game Releases", strings.ToUpper(console)),
		CardText:         strings.Replace(cardText, " und ", "\n", -1),
		CardImage: struct {
			SmallImageUrl string
			LargeImageUrl string
		}{
			"https://images.unsplash.com/photo-1521484358791-8c8504da415e?ixlib=rb-0.3.5&s=297fdee304c29c6474a47682aa8f651b",
			"https://images.unsplash.com/photo-1521484358791-8c8504da415e?ixlib=rb-0.3.5&s=297fdee304c29c6474a47682aa8f651b",
		},
	}

	return alexa.NewResponse(alexaResponse), nil
}

func alexaHandler(req alexa.Request) (*alexa.Response, error) {
	var console, intent string

	requestType := req.RequestBody.Type

	if requestType == "LaunchRequest" {
		return launch()
	}

	if requestType == "IntentRequest" {
		intent = req.RequestBody.Intent.Name
	}

	if intent == "AMAZON.HelpIntent" {
		return help()
	}

	if len(req.RequestBody.Intent.Slots["TYPE_OF_CONSOLE"].Resolutions.ResolutionsPerAuthority) <= 0 {
		return notFound(console)
	}

	if req.RequestBody.Intent.Slots["TYPE_OF_CONSOLE"].Resolutions.ResolutionsPerAuthority[0].Status.Code == "ER_SUCCESS_MATCH" {
		console = req.RequestBody.Intent.Slots["TYPE_OF_CONSOLE"].Resolutions.ResolutionsPerAuthority[0].Values[0].Value.Name
	} else {
		return notFound(console)
	}

	start, end, err := calculateWeekDays(intent)
	if err != nil {
		return nil, err
	}

	cfg, err := newConfig()
	if err != nil {
		return nil, err
	}

	switch {
	case intent == "ReleasePreviousWeek":
		return cfg.Release(intent, console, *start, *end)
	case intent == "ReleaseThisWeek":
		return cfg.Release(intent, console, *start, *end)
	case intent == "ReleaseNextWeek":
		return cfg.Release(intent, console, *start, *end)
	default:
		return notFound(console)
	}
}

func launch() (*alexa.Response, error) {
	var welcomeSpeech = `
	Willkommen beim Skill Game Release!
	Du kannst mich nach aktuellen Game Releases für die Konsolen PS4, PC, XBox One, Nintendo Switch oder den PC fragen.
	Um herauszufinden was du mich alles Fragen kannst: sag einfach "Alexa, frage Game Release was kann ich fragen".
	`

	var welcomeText = `Willkommen beim Skill "Game Release"!\n
Du kannst mich nach aktuellen Game Releases für die Konsolen PS4, PC, XBox One, Nintendo Switch oder den PC fragen.\n
Um herauszufinden was du mich alles Fragen kannst: sag einfach "Alexa, frage Game Release was kann ich fragen".
	`
	launchResponse := &alexa.StandardResponse{
		OutputSpeechText: welcomeSpeech,
		CardTitle:        "Game Release",
		CardText:         welcomeText,
		CardImage: struct {
			SmallImageUrl string
			LargeImageUrl string
		}{
			"https://images.unsplash.com/photo-1521484358791-8c8504da415e?ixlib=rb-0.3.5&s=297fdee304c29c6474a47682aa8f651b",
			"https://images.unsplash.com/photo-1521484358791-8c8504da415e?ixlib=rb-0.3.5&s=297fdee304c29c6474a47682aa8f651b",
		},
	}
	return alexa.NewResponse(launchResponse), nil

}

func help() (*alexa.Response, error) {
	var helpText = `Du kannst mich z.B. folgendes Fragen:\n
"Welche Spiele sind letzte Woche für die Switch, PS4, XBox One oder den PC erschienen?",\n
"Was kommt nächste Woche für die Switch, PS4, XBox One oder den PC?" oder \n
"Was erscheint diese Woche für die Switch, PS4, Xbox One oder den PC?".
`

	var helpSpeech = `
Du kannst mich z.B. folgendes fragen:
"Welche Spiele sind letzte Woche für die Switch, PS4, XBox One oder den PC erschienen?",
"Was kommt nächste Woche für die Switch, PS4, XBox One, oder den PC?" oder
"Was erscheint diese Woche für die Switch, PS4, XBox One oder den PC?".
`
	helpResponse := &alexa.StandardResponse{
		OutputSpeechText: helpSpeech,
		CardTitle:        "Game Release",
		CardText:         helpText,
		CardImage: struct {
			SmallImageUrl string
			LargeImageUrl string
		}{
			"https://images.unsplash.com/photo-1521484358791-8c8504da415e?ixlib=rb-0.3.5&s=297fdee304c29c6474a47682aa8f651b",
			"https://images.unsplash.com/photo-1521484358791-8c8504da415e?ixlib=rb-0.3.5&s=297fdee304c29c6474a47682aa8f651b",
		},
	}
	return alexa.NewResponse(helpResponse), nil
}

func notFound(console string) (*alexa.Response, error) {
	var helpText = `Ich konnte die Plattform %s leider nicht finden, aktuell unterstütze ich nur Switch, PS4, Xbox One und PC.\n
Für Verbesserungswünsche kannst du mir Feedback per Email an hello@juni.io schicken.`

	var helpSpeech = `
		Ich konnte die Plattform %s leider nicht finden. Aktuell unterstütze ich nur Switch, PS4, Xbox One und PC.
		Für Verbesserungswünsche kannst du mir Feedback per Email an hello@juni.io schicken.
		`
	helpResponse := &alexa.StandardResponse{
		OutputSpeechText: fmt.Sprintf(helpSpeech, console),
		CardTitle:        "Game Release",
		CardText:         fmt.Sprintf(helpText, console),
		CardImage: struct {
			SmallImageUrl string
			LargeImageUrl string
		}{
			"https://images.unsplash.com/photo-1521484358791-8c8504da415e?ixlib=rb-0.3.5&s=297fdee304c29c6474a47682aa8f651b",
			"https://images.unsplash.com/photo-1521484358791-8c8504da415e?ixlib=rb-0.3.5&s=297fdee304c29c6474a47682aa8f651b",
		},
	}
	return alexa.NewResponse(helpResponse), nil
}
func calculateWeekDays(intent string) (*string, *string, error) {
	var start, end string
	year, week := time.Now().ISOWeek()
	w, err := goweek.NewWeek(year, week)
	if err != nil {
		return nil, nil, err
	}
	switch {
	case intent == "ReleaseThisWeek":
		start = fmt.Sprintf("%d", w.Days[0].UnixNano()/1000000)
		end = fmt.Sprintf("%d", w.Days[6].UnixNano()/1000000)
	case intent == "ReleaseNextWeek":
		w, err = w.Next()
		if err != nil {
			return nil, nil, err
		}
		start = fmt.Sprintf("%d", w.Days[0].UnixNano()/1000000)
		end = fmt.Sprintf("%d", w.Days[6].UnixNano()/1000000)
	case intent == "ReleasePreviousWeek":
		w, err = w.Previous()
		if err != nil {
			return nil, nil, err
		}
		start = fmt.Sprintf("%d", w.Days[0].UnixNano()/1000000)
		end = fmt.Sprintf("%d", w.Days[6].UnixNano()/1000000)
	}
	return &start, &end, nil
}

func main() {
	lambda.Start(alexaHandler)
}

func removeDuplicates(games []string) []string {
	encountered := map[string]bool{}

	// Create a map of all unique games.
	for v := range games {
		encountered[games[v]] = true
	}

	// Place all keys from the map into a slice.
	result := []string{}
	for key := range encountered {
		result = append(result, key)
	}
	return result
}

func newConfig() (*Config, error) {
	key := os.Getenv("IGDB_KEY")
	if key == "" {
		return nil, errors.New("Failed to fetch IGDB API Key")
	}
	config := &Config{}
	config.APIKey = key
	return config, nil
}

type Option func(*options) error

type options struct {
	Values url.Values
}

func composeValues(opts ...Option) *options {
	o := &options{Values: url.Values{}}

	for _, opt := range opts {
		opt(o)
	}
	return o
}

func addValue(key, value string) Option {
	return func(o *options) error {
		o.Values.Set(key, value)
		return nil
	}
}

func findPlatformID(console string) string {
	switch {
	case console == "ps4":
		return PS4ID
	case console == "switch":
		return NintendoSwitchID
	case console == "xbox one":
		return XboxOneID
	case console == "pc":
		return PCID
	default:
		return fmt.Sprintf("%s,%s,%s,%s", PS4ID, NintendoSwitchID, XboxOneID, PCID)
	}
}
