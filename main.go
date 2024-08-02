package main

import (
	"bytes"
	"fmt"
	"image/color"
	"image/jpeg"
	"log"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/healthcheck"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/gofiber/template/html/v2"
)

func main() {
	// Create a new engine
	engine := html.New("./views", ".html")

	// Create a new Fiber app with the Pug engine
	app := fiber.New(fiber.Config{
		Views: engine,
	})

	//security middleware
	app.Use(cors.New())
	app.Use(helmet.New())
	app.Use(limiter.New(limiter.Config{
		Next: func(c *fiber.Ctx) bool {
			return c.IP() == "127.0.0.1"
		},
		Max:        20,
		Expiration: 30 * time.Second,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.Get("x-forwarded-for")
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).SendString("Too many requests, slow down")
		},
	}))
	app.Use(logger.New())
	app.Use(cache.New())

	app.Use(recover.New())
	app.Use(compress.New())

	app.Use(healthcheck.New())
	app.Get("/monitor", monitor.New(monitor.Config{
		Title: "Image Placeholder Service Monitor",
	}))

	// Serve static files
	app.Static("/", "./public")

	// Route for the home page
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Render("index", nil)
	})
	// Route for the documentation page
	app.Get("/docs", func(c *fiber.Ctx) error {
		return c.Render("docs", nil)
	})

	// Route for generating images
	app.Get("/:format", generateImage)
	app.Get("/:format/:width", generateImage)
	app.Get("/:format/:width/:height", generateImage)
	app.Get("/:format/:width/:height/:text", generateImage)
	app.Get("/:format/:width/:height/:text/:font", generateImage)

	log.Fatal(app.Listen(":4000"))
}

func generateImage(c *fiber.Ctx) error {
	format := strings.ToLower(c.Params("format"))
	width, err := strconv.Atoi(c.Params("width"))
	if err != nil || width <= 0 {
		width = 150
	}

	height, err := strconv.Atoi(c.Params("height"))
	if err != nil || height <= 0 {
		height = width
	}

	//size limit
	if width > 50000 || height > 50000 {
		return c.Status(fiber.StatusBadRequest).SendString("Width and height must be less than 50000")
	}

	text := c.Params("text")
	if text == "_" || text == "" {
		text = fmt.Sprintf("%dx%d", width, height)
	} else {
		text, _ = url.QueryUnescape(text)
	}

	bgColor := c.Query("bg", "E5E5E5")
	borderColor := c.Query("border", "000000")
	textColor := c.Query("textcolor", "A0A0A0")

	switch format {
	case "svg":
		svg := generateSVGContent(width, height, text, bgColor, borderColor, textColor)
		c.Set("Content-Type", "image/svg+xml")
		return c.SendString(svg)
	case "png":
		return generateImageContent(c, "png", width, height, text, bgColor, borderColor, textColor)
	case "jpg":
		return generateImageContent(c, "jpg", width, height, text, bgColor, borderColor, textColor)
	case "jpeg":
		return generateImageContent(c, "jpeg", width, height, text, bgColor, borderColor, textColor)
	default:
		return c.Status(fiber.StatusBadRequest).SendString("Unsupported format, available formats: svg, png, jpeg")
	}
}

func generateSVGContent(width, height int, text, bgColor, borderColor, textColor string) string {
	fontSize := int(math.Min(float64(width), float64(height)) * 0.1)

	svgTemplate := `<svg width="%d" height="%d" xmlns="http://www.w3.org/2000/svg">
		<style type="text/css"></style>
		<rect width="100%%" height="100%%" fill="#%s" stroke="#%s" stroke-width="1"/>
		<text x="50%%" y="50%%" dominant-baseline="middle" text-anchor="middle" font-family="Arial" font-size="%d" fill="#%s">%s</text>
	</svg>`

	return fmt.Sprintf(svgTemplate, width, height, bgColor, borderColor, fontSize, textColor, text)
}

func generateImageContent(c *fiber.Ctx, format string, width, height int, text, bgColor, borderColor, textColor string) error {
	dc := gg.NewContext(width, height)

	bgColorParsed, err := parseHexColor(bgColor)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Invalid background color")
	}
	dc.SetColor(bgColorParsed)
	dc.Clear()

	borderColorParsed, err := parseHexColor(borderColor)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Invalid border color")
	}
	dc.SetColor(borderColorParsed)
	dc.DrawRectangle(0, 0, float64(width), float64(height))
	dc.Stroke()

	fontSize := int(math.Min(float64(width), float64(height)) * 0.1)
	dc.SetRGB(0, 0, 0)
	dc.LoadFontFace("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", float64(fontSize))
	dc.FontHeight()
	textColorParsed, err := parseHexColor(textColor)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Invalid text color")
	}
	dc.SetColor(textColorParsed)
	dc.DrawStringAnchored(text, float64(width)/2, float64(height)/2, 0.5, 0.5)

	var buf bytes.Buffer
	switch format {
	case "png":
		c.Set("Content-Type", "image/png")
		err = dc.EncodePNG(&buf)
	case "jpeg":
		c.Set("Content-Type", "image/jpeg")
		err = jpeg.Encode(&buf, dc.Image(), nil)
	case "jpg":
		c.Set("Content-Type", "image/jpeg")
		err = jpeg.Encode(&buf, dc.Image(), nil)
	default:
		return c.Status(fiber.StatusBadRequest).SendString("Unsupported format")
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to encode image")
	}

	return c.SendStream(&buf)
}

func parseHexColor(s string) (color.Color, error) {
	c := color.RGBA{A: 0xff}
	switch len(s) {
	case 6:
		_, err := fmt.Sscanf(s, "%02x%02x%02x", &c.R, &c.G, &c.B)
		return c, err
	case 8:
		_, err := fmt.Sscanf(s, "%02x%02x%02x%02x", &c.R, &c.G, &c.B, &c.A)
		return c, err
	default:
		return c, fmt.Errorf("invalid length, must be 6 or 8")
	}
}
