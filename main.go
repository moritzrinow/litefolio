package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/gomail.v2"
)

const DefaultAddress = "0.0.0.0:8080"
const DefaultSmtpPort = 587
const DefaultMaxMessageLength = 4096
const MaxEmailSubjectLength = 50

const TurnstileSiteVerifyUrl = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

const DefaultAccentColor = "amber"

//go:embed VERSION
var version string

type Skill struct {
	Name string `yaml:"name"`
	Logo string `yaml:"logo"`
}

type Experience struct {
	Title       string        `yaml:"title"`
	Company     string        `yaml:"company"`
	CompanyUrl  string        `yaml:"companyUrl"`
	Period      string        `yaml:"period"`
	Description template.HTML `yaml:"description"`
}

type Social struct {
	Name string `yaml:"name"`
	Url  string `yaml:"url"`
}

type Portfolio struct {
	Name               string        `yaml:"name"`
	Email              string        `yaml:"email"`
	Phone              string        `yaml:"phone"`
	Title              string        `yaml:"title"`
	Subtitle           string        `yaml:"subtitle"`
	AboutMe            template.HTML `yaml:"aboutMe"`
	Skills             []Skill       `yaml:"skills"`
	Experience         []Experience  `yaml:"experience"`
	Socials            []Social      `yaml:"socials"`
	Description        string        `yaml:"description"`
	ContactDescription template.HTML `yaml:"contactDescription"`
	Style              template.CSS  `yaml:"style"`
	AccentColor        template.CSS  `yaml:"accentColor"`
	FontFamily         template.CSS  `yaml:"fontFamily"`
	BlogUrl            string        `yaml:"blogUrl"`
	PictureUrl         string        `yaml:"pictureUrl"`
	FaviconUrl         string        `yaml:"faviconUrl"`
}

type Config struct {
	Address            string        `yaml:"address"`
	BaseUrl            string        `yaml:"baseUrl"`
	TurnstileDisabled  bool          `yaml:"turnstileDisabled"`
	TurnstileSiteKey   string        `yaml:"turnstileSiteKey"`
	TurnstileSecretKey string        `yaml:"turnstileSecretKey"`
	SmtpEnabled        bool          `yaml:"smtpEnabled"`
	SmtpHost           string        `yaml:"smtpHost"`
	SmtpPort           int           `yaml:"smtpPort"`
	SmtpUser           string        `yaml:"smtpUser"`
	SmtpPassword       string        `yaml:"smtpPassword"`
	SmtpFrom           string        `yaml:"smtpFrom"`
	MaxMessageLength   int           `yaml:"maxMessageLength"`
	Head               template.HTML `yaml:"head"`
	CreditProject      bool          `yaml:"creditProject"`
}

type TemplateCtx struct {
	C *Config
	P *Portfolio
}

var (
	vc            *viper.Viper
	vp            *viper.Viper
	configFile    string
	portfolioFile string
)

var serverCmd = &cobra.Command{
	Use:   "litefolio",
	Short: "Minimalistic developer portfolio website",
	RunE: func(cmd *cobra.Command, args []string) error {
		return Run(cmd)
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of litefolio",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}

func init() {
	gin.SetMode(gin.ReleaseMode)

	serverCmd.Root().CompletionOptions.DisableDefaultCmd = true

	serverCmd.AddCommand(versionCmd)

	vc = viper.New()
	vp = viper.New()

	serverCmd.Flags().StringVarP(&configFile, "config", "c", "litefolio.yaml", "Path to config file")
	serverCmd.Flags().StringVarP(&portfolioFile, "portfolio", "p", "portfolio.yaml", "Path to portfolio file")
	serverCmd.Flags().String("address", DefaultAddress, "IP and port the server should bind to")
	serverCmd.Flags().String("base-url", "", "Base-URL")
	serverCmd.Flags().Bool("turnstile-disabled", false, "Whether Turnstile is disabled (use with caution)")
	serverCmd.Flags().String("turnstile-site-key", "", "Turnstile Site Key")
	serverCmd.Flags().String("turnstile-secret-key", "", "Turnstile Secret Key")
	serverCmd.Flags().Bool("smtp-enabled", false, "Whether SMTP is enabled for the contact-form")
	serverCmd.Flags().String("smtp-host", "", "SMTP hostname (contact-form)")
	serverCmd.Flags().Int("smtp-port", DefaultSmtpPort, "SMTP port (contact-form)")
	serverCmd.Flags().String("smtp-user", "", "SMTP username (contact-form)")
	serverCmd.Flags().String("smtp-password", "", "SMTP password (contact-form)")
	serverCmd.Flags().String("smtp-from", "", "SMTP From-Address (contact-form)")
	serverCmd.Flags().Int("max-message-length", DefaultMaxMessageLength, "Max message size (contact-form)")

	vc.SetDefault("Address", DefaultAddress)
	vc.SetDefault("BaseUrl", "")
	vc.SetDefault("SmtpPort", DefaultSmtpPort)
	vc.SetDefault("MaxMessageLength", DefaultMaxMessageLength)
	vc.BindPFlag("Address", serverCmd.Flags().Lookup("address"))
	vc.BindPFlag("BaseUrl", serverCmd.Flags().Lookup("base-url"))
	vc.BindPFlag("TurnstileDisabled", serverCmd.Flags().Lookup("turnstile-disabled"))
	vc.BindPFlag("TurnstileSiteKey", serverCmd.Flags().Lookup("turnstile-site-key"))
	vc.BindPFlag("TurnstileSecretKey", serverCmd.Flags().Lookup("turnstile-secret-key"))
	vc.BindPFlag("SmtpEnabled", serverCmd.Flags().Lookup("smtp-enabled"))
	vc.BindPFlag("SmtpHost", serverCmd.Flags().Lookup("smtp-host"))
	vc.BindPFlag("SmtpPort", serverCmd.Flags().Lookup("smtp-port"))
	vc.BindPFlag("SmtpUser", serverCmd.Flags().Lookup("smtp-user"))
	vc.BindPFlag("SmtpPassword", serverCmd.Flags().Lookup("smtp-password"))
	vc.BindEnv("TurnstileSiteKey", "LF_TURNSTILE_SITE_KEY")
	vc.BindEnv("TurnstileSecretKey", "LF_TURNSTILE_SECRET_KEY")
	vc.BindEnv("SmtpUser", "LF_SMTP_USER")
	vc.BindEnv("SmtpPassword", "LF_SMTP_PASSWORD")
	vc.SetConfigType("yaml")

	vp.SetDefault("AccentColor", DefaultAccentColor)
	vp.SetConfigType("yaml")
}

func Run(cmd *cobra.Command) error {
	vc.SetConfigFile(configFile)
	vp.SetConfigFile(portfolioFile)

	if err := vc.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}

	var cfg Config
	if err := vc.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("error loading config: %v", err)
	}

	if err := vp.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading portfolio file: %v", err)
	}

	var p Portfolio
	if err := vp.Unmarshal(&p); err != nil {
		return fmt.Errorf("error loading portfilio: %v", err)
	}

	return RunServer(cmd.Context(), &cfg, &p)
}

func RunServer(ctx context.Context, cfg *Config, p *Portfolio) error {
	if strings.HasSuffix(cfg.BaseUrl, "/") {
		return fmt.Errorf("base-url must not end with '/'")
	}

	router := gin.Default()

	groupRoute := cfg.BaseUrl
	if groupRoute == "" {
		groupRoute = "/"
	}

	group := router.Group(groupRoute)

	group.StaticFile("favicon.png", "./assets/favicon.png")

	group.StaticFile("robots.txt", "./assets/robots.txt")

	group.StaticFile("styles/app.css", "./styles/output.css")

	group.Static("/assets", "./assets")

	router.FuncMap["year"] = func() int {
		return time.Now().UTC().Year()
	}

	router.LoadHTMLGlob("templates/*")

	group.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", TemplateCtx{C: cfg, P: p})
	})

	group.POST("/contact", func(c *gin.Context) {
		if !cfg.SmtpEnabled {
			c.AbortWithError(http.StatusUnprocessableEntity, fmt.Errorf("SMTP is disabled"))

			return
		}

		if !cfg.TurnstileDisabled {
			tsResponse := c.PostForm("cf-turnstile-response")
			if tsResponse == "" {
				c.HTML(http.StatusOK, "send-message-error.html", gin.H{
					"Error": "Please verify you are human",
				})

				return
			}

			body := map[string]any{
				"secret":   cfg.TurnstileSecretKey,
				"response": tsResponse,
				"remoteip": c.RemoteIP(),
			}

			json, _ := json.Marshal(body)

			resp, err := http.Post(TurnstileSiteVerifyUrl, "application/json", bytes.NewBuffer(json))
			if err != nil {
				c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("invalid captcha response"))

				return

			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				c.HTML(http.StatusOK, "send-message-error.html", gin.H{
					"Error": "Invalid captcha response. Please try again.",
				})

				return
			}
		}

		name := c.PostForm("name")

		email := c.PostForm("email")

		message := c.PostForm("message")

		if message == "" {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("invalid message"))

			return
		}

		if len(message) > cfg.MaxMessageLength {
			c.AbortWithError(http.StatusRequestEntityTooLarge, fmt.Errorf("message too long"))

			return
		}

		var subject string

		if len(message) > MaxEmailSubjectLength {
			subject = message[:MaxEmailSubjectLength] + "..."
		} else {
			subject = message
		}

		m := gomail.NewMessage()

		m.SetHeader("From", m.FormatAddress(cfg.SmtpFrom, name))
		m.SetHeader("To", p.Email)
		m.SetHeader("Subject", subject)
		m.SetHeader("Reply-To", m.FormatAddress(email, name))

		m.SetBody("text/html", fmt.Sprintf("<p>%s</p>", message))

		d := gomail.NewDialer(cfg.SmtpHost, cfg.SmtpPort, cfg.SmtpUser, cfg.SmtpPassword)

		err := d.DialAndSend(m)

		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("error sending email: %v", err))

			return
		}

		c.HTML(http.StatusOK, "send-message-success.html", gin.H{})
	})

	slog.Info("Running server", "address", cfg.Address)

	router.Run(cfg.Address)

	return nil
}

func main() {
	err := serverCmd.Execute()

	if err != nil {
		panic(err)
	}
}
