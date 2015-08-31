package main

import (
    "log"
    "time"
    "net/http"
    "encoding/json"
    "strings"
)

type weatherProvider interface{
    temperature(city string) (float64, error) // in Kelvin, naturally
}

type openWeatherMap struct{}

func (w openWeatherMap) temperature(city string) (float64, error){
    resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?q="+city)
    if err != nil{
        return 0, err
    }

    defer resp.Body.Close()
    var d struct{
        Main struct{
            Kelvin float64 `json:"temp"`
        }
    }

    if err := json.NewDecoder(resp.Body).Decode(&d); err != nil{
        return 0, err
    }

    log.Printf("openWeatherMap: %s: %.2f", city, d.Main.Kelvin)
    return d.Main.Kelvin, nil
}

type forecastIo struct{
    apikey string
}

var placeLatitudeLongitude = map[string] string{
    "tokyo": "35.69,139.69",
}

func (w forecastIo) temperature(place string) (float64, error){
    resp, err := http.Get("https://api.forecast.io/forecast/" + w.apikey + "/" +placeLatitudeLongitude[place])
    if err != nil{
        return 0, err
    }

    defer resp.Body.Close()
    var d struct{
        Currently struct{
            Temperature float64 `json:"temperature"`
        }`json:"currently"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&d); err != nil{
        return 0, err
    }

    kelvin := 5.0 / 9.0 * (d.Currently.Temperature - 32) + 273.15
    log.Printf("forecast.io: %s[%s]: %.2f", place, placeLatitudeLongitude[place], kelvin)
    return kelvin, nil
}

type weatherUnderground struct{
    apikey string
}

func (w weatherUnderground) temperature(city string) (float64, error){
    resp, err := http.Get("http://api.wunderground.com/api/" + w.apikey + "/conditions/q/" + city + ".json")
    if err != nil{
        return 0, err
    }

    defer resp.Body.Close()
    var d struct{
        Observation struct{
            Celsius float64 `json:"temp_c"`
        }`json:"current_observation"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&d); err != nil{
        return 0, err
    }

    kelvin := d.Observation.Celsius + 273.15
    log.Printf("weatherUnderground: %s: %.2f", city, kelvin)
    return kelvin, nil
}

type weatherData struct{
    Name string `json:"name"`
    Main struct{
        Kelvin float64 `json:"temp"`
    }`json:"main"`
}

type multiWeatherProvider []weatherProvider

func (w multiWeatherProvider) temperature(city string) (float64, error){
    // Make a channel for temperatures, and a channel for errors.
    // Each provider will push a value with into only one.
    temps := make(chan float64, len(w))
    errs := make(chan error, len(w))

    // For each provider, spawn a goroutine with an anonymous function.
    // That function will invoke the temperature method, and forward the response.
    for _, provider := range w{
        go func(p weatherProvider){
            k, err := p.temperature(city)
            if err != nil{
                errs <- err
                return
            }
            temps <- k
        }(provider)
    }

    sum := 0.0

    // Collect a temperature on an error from each provider
    for i := 0; i < len(w); i++{
        select{
        case temp := <-temps:
            sum += temp
        case err := <-errs:
            return 0, err
        }
    }

    // Return the average, same as before
    return sum / float64(len(w)), nil
}

func query(city string)(weatherData, error){
    resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?q=" + city)
    if err != nil{
        return weatherData{}, err
    }

    defer resp.Body.Close()
    var d weatherData

    if err := json.NewDecoder(resp.Body).Decode(&d); err != nil{
        return weatherData{}, err
    }

    return d, nil
}

func main(){
    mw := multiWeatherProvider{
        openWeatherMap{},
        weatherUnderground{apikey: "b0a2ad7d54f9d7bb"},
        forecastIo{apikey: "e00af499915a73555cec54f8cf444155"},
    }

    http.HandleFunc("/hello", hello)
    http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request){
        begin := time.Now()
        city := strings.SplitN(r.URL.Path, "/", 3)[2]

        temp, err := mw.temperature(city)
        if err != nil{
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "city": city,
            "temp": temp,
            "took": time.Since(begin).String(),
        })
    })
    http.ListenAndServe(":8080", nil)
}

func hello(w http.ResponseWriter, r *http.Request){
    w.Write([]byte("hello!"))
}
