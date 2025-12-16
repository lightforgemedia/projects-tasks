package demo

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

type Server struct {
	BaseURL string
	ln      net.Listener
	srv     *http.Server
}

func Start(addr string) (*Server, error) {
	mux := http.NewServeMux()
	registerRoutes(mux)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	s := &Server{
		BaseURL: "http://" + ln.Addr().String(),
		ln:      ln,
		srv: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 2 * time.Second,
		},
	}

	go func() {
		_ = s.srv.Serve(ln)
	}()
	return s, nil
}

func (s *Server) Close(ctx context.Context) error {
	if s == nil || s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/products?query=socks", http.StatusFound)
	})

	mux.HandleFunc("/products", func(w http.ResponseWriter, r *http.Request) {
		writeHTML(w, productsHTMLFor(r.URL.Query().Get("drift")))
	})

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		writeHTML(w, loginHTML)
	})

	mux.HandleFunc("/settings/profile", func(w http.ResponseWriter, r *http.Request) {
		writeHTML(w, settingsProfileHTML)
	})

	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		writeHTML(w, dashboardHTML)
	})
}

func writeHTML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func productsHTMLFor(drift string) string {
	quickAdd := `<button type="button" aria-label="Quick Add" data-testid="quick-add">Quick Add</button>`
	switch drift {
	case "cosmetic":
		quickAdd = `<button type="button" aria-label="Quick-Add" data-testid="quick-add">Quick-Add</button>`
	case "structural":
		// Role changes from button -> link, while keeping the same name.
		quickAdd = `<a href="#" aria-label="Quick Add" data-testid="quick-add">Quick Add</a>`
	}
	return fmt.Sprintf(productsHTMLTemplate, quickAdd)
}

const productsHTMLTemplate = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Products</title>
    <style>
      body { font-family: system-ui, sans-serif; padding: 24px; }
      #mini-cart { border: 1px solid #ddd; padding: 12px; width: 280px; display: none; }
      #mini-cart[aria-hidden="false"] { display: block; }
    </style>
  </head>
  <body>
    <h1>Products</h1>
    <p>Query: <span data-testid="query"></span></p>

    %s
    <div role="dialog" aria-label="Mini cart" data-testid="mini-cart" id="mini-cart" aria-hidden="true">
      <strong>Mini cart</strong>
      <div data-testid="cart-items">1 item</div>
    </div>

    <script>
      (function () {
        var qp = new URLSearchParams(location.search || "");
        var q = qp.get("query") || "";
        var el = document.querySelector('[data-testid="query"]');
        if (el) el.textContent = q;

        var btn = document.querySelector('[data-testid="quick-add"]');
        var cart = document.querySelector('[data-testid="mini-cart"]');
        if (btn && cart) {
          btn.addEventListener("click", function () {
            cart.setAttribute("aria-hidden", "false");
          });
        }
      })();
    </script>
  </body>
</html>`

const loginHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Login</title>
    <style>
      body { font-family: system-ui, sans-serif; padding: 24px; }
      label { display: block; margin-top: 12px; }
      input { width: 320px; padding: 8px; }
    </style>
  </head>
  <body>
    <h1>Login</h1>

    <label for="email">Email</label>
    <input id="email" type="email" autocomplete="off" />

    <label for="password">Password</label>
    <input id="password" type="password" autocomplete="off" />

    <button type="button" aria-label="Sign in" data-testid="sign-in">Sign in</button>

    <script>
      (function () {
        var btn = document.querySelector('[data-testid="sign-in"]');
        if (!btn) return;
        btn.addEventListener("click", function () {
          location.href = "/dashboard";
        });
      })();
    </script>
  </body>
</html>`

const dashboardHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Dashboard</title>
  </head>
  <body>
    <h1>Dashboard</h1>
    <p data-testid="dashboard">ok</p>
  </body>
</html>`

const settingsProfileHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Profile</title>
    <style>
      body { font-family: system-ui, sans-serif; padding: 24px; }
      #modal { border: 1px solid #ddd; padding: 16px; width: 420px; display: none; margin-top: 12px; }
      #modal[aria-hidden="false"] { display: block; }
      #toast { display: none; margin-top: 16px; padding: 10px; background: #e7f7ee; border: 1px solid #b6e3c6; }
      #toast[aria-hidden="false"] { display: block; }
      label { display: block; margin-top: 12px; }
      input { width: 320px; padding: 8px; }
    </style>
  </head>
  <body>
    <h1>Profile</h1>
    <button type="button" aria-label="Edit profile" data-testid="edit-profile">Edit profile</button>

    <div role="dialog" aria-label="Edit profile modal" id="modal" data-testid="profile-modal" aria-hidden="true">
      <label for="display-name">Display name</label>
      <input id="display-name" type="text" />
      <button type="button" aria-label="Save" data-testid="save-profile">Save</button>
    </div>

    <div role="status" aria-label="Toast" id="toast" data-testid="toast" aria-hidden="true">Saved</div>

    <script>
      (function () {
        var edit = document.querySelector('[data-testid="edit-profile"]');
        var modal = document.querySelector('[data-testid="profile-modal"]');
        var save = document.querySelector('[data-testid="save-profile"]');
        var toast = document.querySelector('[data-testid="toast"]');
        if (edit && modal) {
          edit.addEventListener("click", function () {
            modal.setAttribute("aria-hidden", "false");
          });
        }
        if (save && toast) {
          save.addEventListener("click", function () {
            toast.setAttribute("aria-hidden", "false");
          });
        }
      })();
    </script>
  </body>
</html>`
