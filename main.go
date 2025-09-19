package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

)

var OMDB_API_KEY string

//movie responses
type MovieResponse struct {
	Title      string `json:"Title"`
	Year       string `json:"Year"`
	Genre      string `json:"Genre"`
	Plot       string `json:"Plot"`
	IMDBID     string `json:"imdbID"`
	IMDBRating string `json:"imdbRating"`
	Response   string `json:"Response"`
	Error      string `json:"Error,omitempty"`
}


type SearchResults struct {
	Search []struct {
		Title  string `json:"Title"`
		Year   string `json:"Year"`
		IMDBID string `json:"imdbID"`
		Type   string `json:"Type"`
	} `json:"Search"`
	Response string `json:"Response"`
	Error    string `json:"Error,omitempty"`
}

//get movie details
func fetchMovie(params map[string]string) (*MovieResponse, error) {
	baseURL := "http://www.omdbapi.com/"
	query := ""
	for k, v := range params {
		query += fmt.Sprintf("&%s=%s", k, v)
	}
	url := fmt.Sprintf("%s?apikey=%s%s", baseURL, OMDB_API_KEY, query)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var movie MovieResponse
	if err := json.NewDecoder(resp.Body).Decode(&movie); err != nil {
		return nil, err
	}

	if movie.Response == "False" {
		return nil, fmt.Errorf(movie.Error)
	}

	return &movie, nil
}

//get search results
func fetchSearchResults(query string) (*SearchResults, error) {
	baseURL := "http://www.omdbapi.com/"
	url := fmt.Sprintf("%s?apikey=%s&s=%s&type=movie", baseURL, OMDB_API_KEY, query)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var results SearchResults
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	if results.Response == "False" {
		return nil, fmt.Errorf(results.Error)
	}

	return &results, nil
}

// get info
func getMovie(c *gin.Context) {
	title := c.Query("title")
	id := c.Query("id")

	if title == "" && id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide ?title=MovieName or ?id=IMDBid"})
		return
	}

	params := map[string]string{}
	if title != "" {
		params["t"] = title
	}
	if id != "" {
		params["i"] = id
	}

	movie, err := fetchMovie(params)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, movie)
}

//get episodes
func getEpisode(c *gin.Context) {
	seriesTitle := c.Query("series_title")
	season := c.Query("season")
	episode := c.Query("episode_number")

	if seriesTitle == "" || season == "" || episode == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Please provide ?series_title=...&season=1&episode_number=1",
		})
		return
	}

	params := map[string]string{
		"t":       seriesTitle,
		"Season":  season,
		"Episode": episode,
	}

	ep, err := fetchFromOMDb(params)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Clean response
	c.JSON(http.StatusOK, gin.H{
		"Series":     ep.Title,    // OMDb puts series title here
		"Episode":    ep.Episode,  // episode number
		"Season":     ep.Season,   // season number
		"Title":      ep.Title,    // episode title
		"Released":   ep.Released, // release date
		"Plot":       ep.Plot,
		"imdbID":     ep.IMDBID,
		"imdbRating": ep.IMDBRating,
	})
}


//get genre movies
func getMoviesByGenre(c *gin.Context) {
	genre := c.Query("genre")
	if genre == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide a genre using ?genre=GenreName"})
		return
	}

	matchingMovies := []map[string]interface{}{}
	searchSeeds := []string{"the", "a", "love", "man", "girl", "night", "day"}

	for _, seed := range searchSeeds {
		results, err := fetchSearchResults(seed)
		if err != nil {
			continue
		}

		for _, item := range results.Search {
			movie, err := fetchMovie(map[string]string{"i": item.IMDBID})
			if err != nil || movie.Response == "False" || movie.IMDBRating == "N/A" {
				continue
			}

			genres := strings.Split(movie.Genre, ",")
			for _, g := range genres {
				if strings.EqualFold(strings.TrimSpace(g), genre) {
					matchingMovies = append(matchingMovies, map[string]interface{}{
						"Title":      movie.Title,
						"Year":       movie.Year,
						"Genre":      movie.Genre,
						"imdbRating": movie.IMDBRating,
						"imdbID":     movie.IMDBID,
					})
					break
				}
			}
		}
	}

	sort.Slice(matchingMovies, func(i, j int) bool {
		r1, _ := strconv.ParseFloat(matchingMovies[i]["imdbRating"].(string), 64)
		r2, _ := strconv.ParseFloat(matchingMovies[j]["imdbRating"].(string), 64)
		return r1 > r2
	})

	if len(matchingMovies) > 15 {
		matchingMovies = matchingMovies[:15]
	}

	c.JSON(http.StatusOK, matchingMovies)
}

// get recommendations
func getRecommendations(c *gin.Context) {
	fav := c.Query("favorite_movie")
	if fav == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide ?favorite_movie=MovieTitle"})
		return
	}

	// Step 1: Fetch favorite movie details
	favMovie, err := fetchFromOMDb(map[string]string{"t": fav})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Favorite movie not found"})
		return
	}

	// Step 2: Extract metadata
	genres := strings.Split(favMovie.Genre, ",")
	directors := strings.Split(favMovie.Director, ",")
	actors := strings.Split(favMovie.Actors, ",")

	// Helper to collect recommendations
	collect := func(level string, keywords []string, limit int) []gin.H {
		results := []gin.H{}
		seen := map[string]bool{}

		for _, kw := range keywords {
			kw = strings.TrimSpace(kw)
			if kw == "" || kw == "N/A" {
				continue
			}

			search, _ := fetchSearchPage(kw, 1)
			for _, s := range search.Search {
				if seen[s.IMDBID] || s.IMDBID == favMovie.IMDBID {
					continue
				}
				movie, err := fetchFromOMDb(map[string]string{"i": s.IMDBID})
				if err != nil || movie.IMDBRating == "N/A" {
					continue
				}

				seen[s.IMDBID] = true
				results = append(results, gin.H{
					"Title":      movie.Title,
					"Year":       movie.Year,
					"Genre":      movie.Genre,
					"imdbRating": movie.IMDBRating,
					"imdbID":     movie.IMDBID,
					"Why":        level,
				})
			}
		}

		// Sort by IMDb rating
		sort.Slice(results, func(i, j int) bool {
			ri, _ := strconv.ParseFloat(results[i]["imdbRating"].(string), 64)
			rj, _ := strconv.ParseFloat(results[j]["imdbRating"].(string), 64)
			return ri > rj
		})

		if len(results) > limit {
			results = results[:limit]
		}
		return results
	}

	// Step 3: Build hierarchical list
	final := []gin.H{}
	final = append(final, collect("Genre", genres, 20)...)
	final = append(final, collect("Director", directors, 20)...)
	final = append(final, collect("Actor", actors, 20)...)

	// Dedup
	unique := []gin.H{}
	seen := map[string]bool{}
	for _, m := range final {
		id := m["imdbID"].(string)
		if !seen[id] {
			unique = append(unique, m)
			seen[id] = true
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"favorite_movie": favMovie.Title,
		"recommendations": unique,
	})
}


func main() {
	
	OMDB_API_KEY = os.Getenv("OMDB_API_KEY")
	if OMDB_API_KEY == "" {
		panic("set api key")
	}

	router := gin.Default()

	router.GET("/api/movie", getMovie)
	router.GET("/api/episode", getEpisode)
	router.GET("/api/movies/genre", getMoviesByGenre)
	router.GET("/api/movies/recommendations", getRecommendations)

	router.Run(":8080")
}

