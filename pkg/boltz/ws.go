package boltz

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
)

const reconnectInterval = 15 * time.Second
const pingInterval = 30 * time.Second
const pongWait = 5 * time.Second

type SwapStatusResponse struct {
	Status           string `json:"status"`
	ZeroConfRejected bool   `json:"zeroConfRejected"`
	Transaction      struct {
		Id  string `json:"id"`
		Hex string `json:"hex"`
	} `json:"transaction"`

	Error string `json:"error"`
}

type SwapUpdate struct {
	SwapStatusResponse `mapstructure:",squash"`
	Id                 string `json:"id"`
}

type Websocket struct {
	Updates chan SwapUpdate

	apiUrl        string
	subscriptions chan bool
	conn          *websocket.Conn
	closed        bool
	reconnect     bool
	dialer        *websocket.Dialer
	swapIds       []string
}

type wsResponse struct {
	Event   string `json:"event"`
	Error   string `json:"error"`
	Channel string `json:"channel"`
	Args    []any  `json:"args"`
}

func (boltz *Api) NewWebsocket() *Websocket {
	httpTransport, ok := boltz.Client.Transport.(*http.Transport)

	dialer := *websocket.DefaultDialer
	if ok {
		dialer.Proxy = httpTransport.Proxy
	}

	return &Websocket{
		apiUrl:        boltz.WSURL,
		subscriptions: make(chan bool),
		dialer:        &dialer,
		Updates:       make(chan SwapUpdate),
	}
}

func (boltz *Websocket) Connect() error {
	wsUrl, err := url.Parse(boltz.apiUrl)
	if err != nil {
		return err
	}
	wsUrl.Path += "/v2/ws"

	if wsUrl.Scheme == "https" {
		wsUrl.Scheme = "wss"
	} else if wsUrl.Scheme == "http" {
		wsUrl.Scheme = "ws"
	}

	conn, _, err := boltz.dialer.Dial(wsUrl.String(), nil)
	if err != nil {
		return fmt.Errorf("could not connect to boltz ws at %s: %w", wsUrl, err)
	}
	boltz.conn = conn

	setDeadline := func() error {
		return conn.SetReadDeadline(time.Now().Add(pingInterval + pongWait))
	}
	_ = setDeadline()
	conn.SetPongHandler(func(string) error {
		return setDeadline()
	})
	pingTicker := time.NewTicker(pingInterval)

	go func() {
		defer pingTicker.Stop()
		for range pingTicker.C {
			// Will not wait longer with writing than for the response
			err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(pongWait))
			if err != nil {
				if boltz.closed {
					return
				}
				return
			}
		}
	}()

	go func() {
		for {
			msgType, message, err := conn.ReadMessage()
			if err != nil {
				if boltz.closed {
					close(boltz.Updates)
					return
				}
				break
			}

			switch msgType {
			case websocket.TextMessage:
				var response wsResponse
				if err := json.Unmarshal(message, &response); err != nil {
					continue
				}
				fmt.Printf("Received message: %+v\n", response)
				if response.Error != "" {
					continue
				}

				switch response.Event {
				case "update":
					switch response.Channel {
					case "swap.update":
						for _, arg := range response.Args {
							var update SwapUpdate
							if err := mapstructure.Decode(arg, &update); err != nil {
							}
							boltz.Updates <- update
						}
					default:
					}
				case "subscribe":
					boltz.subscriptions <- true
					continue
				default:
				}
			}
		}
		for {
			pingTicker.Stop()
			if boltz.reconnect {
				boltz.reconnect = false
				return
			} else {
				time.Sleep(reconnectInterval)
			}
			err := boltz.Connect()
			if err == nil {
				return
			}
		}
	}()

	if len(boltz.swapIds) > 0 {
		return boltz.subscribe(boltz.swapIds)
	}

	return nil
}

func (boltz *Websocket) subscribe(swapIds []string) error {
	if boltz.closed {
		return errors.New("websocket is closed")
	}
	if len(swapIds) == 0 {
		return nil
	}
	if err := boltz.conn.WriteJSON(map[string]any{
		"op":      "subscribe",
		"channel": "swap.update",
		"args":    swapIds,
	}); err != nil {
		return err
	}
	select {
	case <-boltz.subscriptions:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("no answer from boltz")
	}
}

func (boltz *Websocket) Subscribe(swapIds []string) error {
	if err := boltz.subscribe(swapIds); err != nil {
		// the connection might be dead, so forcefully reconnect
		if err := boltz.Reconnect(); err != nil {
			return fmt.Errorf("could not reconnect boltz ws: %w", err)
		}
		if err := boltz.subscribe(swapIds); err != nil {
			return err
		}
	}
	boltz.swapIds = append(boltz.swapIds, swapIds...)
	return nil
}

func (boltz *Websocket) Unsubscribe(swapId string) {
	boltz.swapIds = slices.DeleteFunc(boltz.swapIds, func(id string) bool {
		return id == swapId
	})
}

func (boltz *Websocket) Close() error {
	boltz.closed = true
	return boltz.conn.Close()
}

func (boltz *Websocket) Reconnect() error {
	if boltz.closed {
		return errors.New("websocket is closed")
	}
	boltz.reconnect = true
	if err := boltz.conn.Close(); err != nil {
	}
	return boltz.Connect()
}
