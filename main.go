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


type MovieResponse struct {
	Title      string `json:"Title"`
	Year       string `json:"Year"`
	Plot       string `json:"Plot"`
	Director   string `json:"Director"`
	Genre      string `json:"Genre"`
	Actors     string `json:"Actors"`
	Country    string `json:"Country"`
	Awards     string `json:"Awards"`
	Season     string `json:"Season,omitempty"`
	Episode    string `json:"Episode,omitempty"`
	Released   string `json:"Released,omitempty"`
	IMDBID     string `json:"imdbID"`
	IMDBRating string `json:"imdbRating"`
	Ratings    []struct {
		Source string `json:"Source"`
		Value  string `json:"Value"`
	} `json:"Ratings"`
	Response string `json:"Response"`
	Error    string `json:"Error,omitempty"`
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


func fetchFromOMDb(params map[string]string, out interface{}) error {
	baseURL := "http://www.omdbapi.com/"
	query := ""
	for k, v := range params {
		query += fmt.Sprintf("&%s=%s", k, v)
	}
	url := fmt.Sprintf("%s?apikey=%s%s", baseURL, OMDB_API_KEY, query)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}

	
	switch v := out.(type) {
	case *MovieResponse:
		if v.Response == "False" {
			return fmt.Errorf(v.Error)
		}
	case *SearchResults:
		if v.Response == "False" {
			return fmt.Errorf(v.Error)
		}
	}
	return nil
}


func fetchMovie(params map[string]string) (*MovieResponse, error) {
	var movie MovieResponse
	if err := fetchFromOMDb(params, &movie); err != nil {
		return nil, err
	}
	return &movie, nil
}

func fetchSearchResults(query string) (*SearchResults, error) {
	return fetchSearchPage(query, 1)
}

func fetchSearchPage(query string, page int) (*SearchResults, error) {
	params := map[string]string{
		"s":    query,
		"type": "movie",
		"page": strconv.Itoa(page),
	}
	var results SearchResults
	if err := fetchFromOMDb(params, &results); err != nil {
		return nil, err
	}
	return &results, nil
}


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

	c.JSON(http.StatusOK, gin.H{
		"Title":    movie.Title,
		"Year":     movie.Year,
		"Plot":     movie.Plot,
		"Director": movie.Director,
		"Country":  movie.Country,
		"Awards":   movie.Awards,
		"Ratings":  movie.Ratings,
	})
}


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

	ep, err := fetchMovie(params)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"Series":     seriesTitle,
		"Episode":    ep.Episode,
		"Season":     ep.Season,
		"Title":      ep.Title,
		"Released":   ep.Released,
		"Plot":       ep.Plot,
		"Country":    ep.Country,
		"Awards":     ep.Awards,
		"imdbID":     ep.IMDBID,
		"imdbRating": ep.IMDBRating,
	})
}


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
			if err != nil || movie.IMDBRating == "N/A" {
				continue
			}

			genres := strings.Split(movie.Genre, ",")
			for _, g := range genres {
				if strings.EqualFold(strings.TrimSpace(g), genre) {
					matchingMovies = append(matchingMovies, map[string]interface{}{
						"Title":      movie.Title,
						"Year":       movie.Year,
						"Genre":      movie.Genre,
						"Country":    movie.Country,
						"Awards":     movie.Awards,
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


func getRecommendations(c *gin.Context) {
	fav := c.Query("favorite_movie")
	if fav == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide ?favorite_movie=MovieTitle"})
		return
	}

	favMovie, err := fetchMovie(map[string]string{"t": fav})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Favorite movie not found"})
		return
	}

	genres := strings.Split(favMovie.Genre, ",")
	directors := strings.Split(favMovie.Director, ",")
	actors := strings.Split(favMovie.Actors, ",")

	seen := map[string]bool{favMovie.IMDBID: true}

	collect := func(level string, keywords []string, limit int) []gin.H {
		results := []gin.H{}
		for _, kw := range keywords {
			kw = strings.TrimSpace(kw)
			if kw == "" || kw == "N/A" {
				continue
			}

			for page := 1; page <= 3 && len(results) < limit; page++ {
				search, err := fetchSearchPage(kw, page)
				if err != nil || search == nil {
					continue
				}

				for _, s := range search.Search {
					if seen[s.IMDBID] {
						continue
					}
					movie, err := fetchMovie(map[string]string{"i": s.IMDBID})
					if err != nil || movie.IMDBRating == "N/A" {
						continue
					}
					seen[s.IMDBID] = true
					results = append(results, gin.H{
						"Title":      movie.Title,
						"Year":       movie.Year,
						"Genre":      movie.Genre,
						"Country":    movie.Country,
						"Awards":     movie.Awards,
						"imdbRating": movie.IMDBRating,
						"imdbID":     movie.IMDBID,
						"Why":        level,
					})
					if len(results) >= limit {
						break
					}
				}
			}
			if len(results) >= limit {
				break
			}
		}
		sort.Slice(results, func(i, j int) bool {
			ri, _ := strconv.ParseFloat(results[i]["imdbRating"].(string), 64)
			rj, _ := strconv.ParseFloat(results[j]["imdbRating"].(string), 64)
			return ri > rj
		})
		return results
	}

	
	genreRecs := collect("Genre", genres, 5)
	directorRecs := collect("Director", directors, 5)
	actorRecs := collect("Actor", actors, 5)

	c.JSON(http.StatusOK, gin.H{
		"favorite_movie": favMovie.Title,
		"recommendations": gin.H{
			"by_genre":    genreRecs,
			"by_director": directorRecs,
			"by_actor":    actorRecs,
		},
	})
}





func main() {
	OMDB_API_KEY = os.Getenv("OMDB_API_KEY")
	if OMDB_API_KEY == "" {
		panic("set OMDB_API_KEY in your environment")
	}

	router := gin.Default()

	router.GET("/api/movie", getMovie)
	router.GET("/api/episode", getEpisode)
	router.GET("/api/movies/genre", getMoviesByGenre)
	router.GET("/api/movies/recommendations", getRecommendations)

	router.Run(":8080")
}
