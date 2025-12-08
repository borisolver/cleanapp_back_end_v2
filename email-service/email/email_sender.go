package email

import (
	"encoding/base64"
	"fmt"
	"image"
	"time"

	"email-service/config"
	"email-service/models"

	"github.com/apex/log"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	reportImgCid = "report_image"
	mapImgCid    = "map_image"
)

// EmailSender handles email sending functionality
type EmailSender struct {
	config *config.Config
	client *sendgrid.Client
}

// NewEmailSender creates a new email sender
func NewEmailSender(cfg *config.Config) *EmailSender {
	client := sendgrid.NewSendClient(cfg.SendGridAPIKey)
	return &EmailSender{
		config: cfg,
		client: client,
	}
}

// SendEmails sends emails to multiple recipients
func (e *EmailSender) SendEmails(recipients []string, reportImage, mapImage []byte) error {
	log.Infof("Sending email to %d recipients", len(recipients))

	var firstErr error
	failed := 0
	for _, recipient := range recipients {
		if err := e.sendOneEmail(recipient, reportImage, mapImage); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			log.Warnf("Error sending email to %s: %v", recipient, err)
			// Continue with other recipients
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d/%d emails failed: %v", failed, len(recipients), firstErr)
	}
	return nil
}

// SendEmailsWithAnalysis sends emails to multiple recipients with analysis data
func (e *EmailSender) SendEmailsWithAnalysis(recipients []string, reportImage, mapImage []byte, analysis *models.ReportAnalysis) error {
	log.Infof("Sending email with analysis to %d recipients", len(recipients))

	var firstErr error
	failed := 0
	for _, recipient := range recipients {
		if err := e.sendOneEmailWithAnalysis(recipient, reportImage, mapImage, analysis); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			log.Warnf("Error sending email to %s: %v", recipient, err)
			// Continue with other recipients
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d/%d emails with analysis failed: %v", failed, len(recipients), firstErr)
	}
	return nil
}

// sendOneEmail sends an email to a single recipient
func (e *EmailSender) sendOneEmail(recipient string, reportImage, mapImage []byte) error {
	from := mail.NewEmail(e.config.SendGridFromName, e.config.SendGridFromEmail)
	subject := "You got a CleanApp report"
	to := mail.NewEmail(recipient, recipient)

	hasReport := len(reportImage) > 0
	hasMap := len(mapImage) > 0

	// Create message
	message := mail.NewV3Mail()
	message.SetFrom(from)
	message.Subject = subject

	p := mail.NewPersonalization()
	p.AddTos(to)
	message.AddPersonalizations(p)

	message.AddContent(mail.NewContent("text/plain", e.getEmailText(recipient, hasReport, hasMap)))
	message.AddContent(mail.NewContent("text/html", e.getEmailHtml(recipient, hasReport, hasMap)))

	if hasReport {
		encodedReportImage := base64.StdEncoding.EncodeToString(reportImage)
		reportAttachment := mail.NewAttachment()
		reportAttachment.SetContent(encodedReportImage)
		reportAttachment.SetType("image/jpeg")
		reportAttachment.SetFilename("report.jpg")
		reportAttachment.SetDisposition("inline")
		reportAttachment.SetContentID(reportImgCid)
		message.AddAttachment(reportAttachment)
	}

	// Add map attachment only if mapImage is provided
	if hasMap {
		encodedMapImage := base64.StdEncoding.EncodeToString(mapImage)
		mapAttachment := mail.NewAttachment()
		mapAttachment.SetContent(encodedMapImage)
		mapAttachment.SetType("image/png")
		mapAttachment.SetFilename("map.png")
		mapAttachment.SetDisposition("inline")
		mapAttachment.SetContentID(mapImgCid)
		message.AddAttachment(mapAttachment)
	}

	// Send email
	start := time.Now()
	response, err := e.client.Send(message)
	if err != nil {
		return err
	}

	duration := time.Since(start)
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		msgID := response.Headers["X-Message-Id"]
		log.Infof("Email accepted by SendGrid for %s (status=%d, id=%s, in %s)", recipient, response.StatusCode, msgID, duration)
		return nil
	}

	body := response.Body
	if len(body) > 512 {
		body = body[:512] + "..."
	}
	return fmt.Errorf("sendgrid returned status %d for %s (in %s): %s", response.StatusCode, recipient, duration, body)
}

// sendOneEmailWithAnalysis sends an email to a single recipient with analysis data
func (e *EmailSender) sendOneEmailWithAnalysis(recipient string, reportImage, mapImage []byte, analysis *models.ReportAnalysis) error {
	from := mail.NewEmail(e.config.SendGridFromName, e.config.SendGridFromEmail)

	// Create subject with analysis title
	subject := "CleanApp Report"
	isDigital := analysis != nil && analysis.Classification == "digital"
	if isDigital {
		subject = "CleanApp alert: major new issue reported for your brand"
	}
	if analysis.Title != "" {
		if isDigital {
			subject = fmt.Sprintf("CleanApp alert: major new issue — %s", analysis.Title)
		} else {
			subject = fmt.Sprintf("CleanApp Report: %s", analysis.Title)
		}
	}

	to := mail.NewEmail(recipient, recipient)

	hasReport := len(reportImage) > 0
	hasMap := len(mapImage) > 0

	// Create message
	message := mail.NewV3Mail()
	message.SetFrom(from)
	message.Subject = subject

	p := mail.NewPersonalization()
	p.AddTos(to)
	message.AddPersonalizations(p)

	message.AddContent(mail.NewContent("text/plain", e.getEmailTextWithAnalysis(recipient, analysis, hasReport, hasMap)))
	message.AddContent(mail.NewContent("text/html", e.getEmailHtmlWithAnalysis(recipient, analysis, hasReport, hasMap)))

	if hasReport {
		encodedReportImage := base64.StdEncoding.EncodeToString(reportImage)
		reportAttachment := mail.NewAttachment()
		reportAttachment.SetContent(encodedReportImage)
		reportAttachment.SetType("image/jpeg")
		reportAttachment.SetFilename("report.jpg")
		reportAttachment.SetDisposition("inline")
		reportAttachment.SetContentID(reportImgCid)
		message.AddAttachment(reportAttachment)
	}

	// Add map attachment only if mapImage is provided
	if hasMap {
		encodedMapImage := base64.StdEncoding.EncodeToString(mapImage)
		mapAttachment := mail.NewAttachment()
		mapAttachment.SetContent(encodedMapImage)
		mapAttachment.SetType("image/png")
		mapAttachment.SetFilename("map.png")
		mapAttachment.SetDisposition("inline")
		mapAttachment.SetContentID(mapImgCid)
		message.AddAttachment(mapAttachment)
	}

	// Send email
	start := time.Now()
	response, err := e.client.Send(message)
	if err != nil {
		return err
	}

	duration := time.Since(start)
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		msgID := response.Headers["X-Message-Id"]
		log.Infof("Email with analysis accepted by SendGrid for %s (status=%d, id=%s, in %s)", recipient, response.StatusCode, msgID, duration)
		return nil
	}
	body := response.Body
	if len(body) > 512 {
		body = body[:512] + "..."
	}
	return fmt.Errorf("sendgrid returned status %d for %s (in %s): %s", response.StatusCode, recipient, duration, body)
}

// addLabel adds text to an image
func (e *EmailSender) addLabel(img *image.RGBA, text string, x, y int) {
	point := fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)}

	d := &font.Drawer{
		Dst:  img,
		Src:  image.Black,
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	d.DrawString(text)
}

// getEmailText returns the plain text content for emails
func (e *EmailSender) getEmailText(recipient string, hasReport, hasMap bool) string {
	sections := ""
	if hasReport || hasMap {
		sections = "\nThis email contains:\n"
		if hasReport {
			sections += "- The report image\n"
		}
		if hasMap {
			sections += "- A map showing the location\n"
		}
	}
	return fmt.Sprintf(`Hello,

You have received a new CleanApp report.%s
Best regards,
The CleanApp Team`, sections)
}

// getEmailHtml returns the HTML content for emails
func (e *EmailSender) getEmailHtml(recipient string, hasReport, hasMap bool) string {
	imagesSection := ""
	if hasReport {
		imagesSection += fmt.Sprintf(`
    <h3>Report Image:</h3>
    <img src="cid:%s" alt="Report Image" style="max-width: 100%%; height: auto;">`, reportImgCid)
	}
	if hasMap {
		imagesSection += fmt.Sprintf(`
    <h3>Location Map:</h3>
    <img src="cid:%s" alt="Map" style="max-width: 100%%; height: auto;">`, mapImgCid)
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>CleanApp Report</title>
</head>
<body>
    <h2>Hello,</h2>
    <p>You have received a new CleanApp report.</p>%s
    <p>Best regards,<br>The CleanApp Team</p>
</body>
</html>`, imagesSection)
}

// getEmailTextWithAnalysis returns the plain text content for emails with analysis data
func (e *EmailSender) getEmailTextWithAnalysis(recipient string, analysis *models.ReportAnalysis, hasReport, hasMap bool) string {
	if analysis.Classification == "digital" {
		digitalSubject := "CleanApp alert: major new issue reported for your brand"
		preheader := "Someone just submitted a brand-related digital report with photos."

		heroReport := ""
		if hasReport {
			heroReport = "\n- Hero: photo of report included."
		}

		heroLocation := ""
		if hasMap {
			heroLocation = "\n- Hero: photo of location included."
		}

		return fmt.Sprintf(`%s
Preheader: %s

Someone just submitted a new digital report mentioning your brand.
CleanApp AI analyzed this issue to highlight potential legal and risk ranges connected to your brand presence.%s%s

AI analysis summary:
- Title: %s
- Description: %s
- Type: Digital Issue

Open the Brand Dashboard to see the AI rationale, mapped areas, and supporting media:
%s

To unsubscribe from these emails, please visit: %s?email=%s
You can also reply to this email with "UNSUBSCRIBE" in the subject line.

Best regards,
The CleanApp Team`,
			digitalSubject,
			preheader,
			heroReport,
			heroLocation,
			analysis.Title,
			analysis.Description,
			e.config.BrandDashboardURL,
			e.config.OptOutURL,
			recipient)
	}

	attachments := ""
	if hasReport || hasMap {
		attachments = "\nThis email contains:\n"
		if hasReport {
			attachments += "- The report image\n"
		}
		if hasMap {
			attachments += "- A map showing the location\n"
		}
		attachments += "- AI analysis results\n"
	}

	return fmt.Sprintf(`Hello,

You have received a new CleanApp report with analysis.

REPORT ANALYSIS:
Title: %s
Description: %s
Type: Physical Issue

PROBABILITY SCORES:
- Litter Probability: %.1f%%
- Hazard Probability: %.1f%%
- Severity Level: %.1f
%s
To unsubscribe from these emails, please visit: %s?email=%s
You can also reply to this email with "UNSUBSCRIBE" in the subject line.

Best regards,
The CleanApp Team`,
		analysis.Title,
		analysis.Description,
		analysis.LitterProbability*100,
		analysis.HazardProbability*100,
		analysis.SeverityLevel,
		attachments,
		e.config.OptOutURL,
		recipient)
}

// getEmailHtmlWithAnalysis returns the HTML content for emails with analysis data
func (e *EmailSender) getEmailHtmlWithAnalysis(recipient string, analysis *models.ReportAnalysis, hasReport, hasMap bool) string {
	isDigital := analysis.Classification == "digital"

	if isDigital {
		subjectLine := "CleanApp alert: major new issue reported for your brand"
		preheader := "Someone just submitted a brand-related digital report. Review the AI analysis and risk ranges."

		reportHero := ""
		if hasReport {
			reportHero = fmt.Sprintf(`
            <div class="hero-card">
                <div class="hero-label">Photo of report</div>
                <img src="cid:%s" alt="Report Image" />
            </div>`, reportImgCid)
		}

		locationHero := ""
		if hasMap {
			locationHero = fmt.Sprintf(`
            <div class="hero-card">
                <div class="hero-label">Photo of location</div>
                <img src="cid:%s" alt="Location Map" />
            </div>`, mapImgCid)
		}

		heroImages := ""
		if reportHero != "" || locationHero != "" {
			heroImages = fmt.Sprintf(`
        <div class="hero-grid">%s%s
        </div>`, reportHero, locationHero)
		}

		return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>%s</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #1f2937; background: #f7f7f8; margin: 0; padding: 0; }
        .preheader { display: none; visibility: hidden; opacity: 0; height: 0; width: 0; overflow: hidden; }
        .container { max-width: 720px; margin: 0 auto; padding: 24px; background: #ffffff; }
        .hero { background: linear-gradient(135deg, #0f766e, #14b8a6); color: #ffffff; padding: 28px; border-radius: 14px; box-shadow: 0 10px 30px rgba(0,0,0,0.12); }
        .eyebrow { text-transform: uppercase; letter-spacing: 0.08em; font-weight: 700; font-size: 12px; margin: 0 0 6px 0; opacity: 0.85; }
        h1 { margin: 0 0 10px 0; font-size: 26px; }
        .subhead { margin: 0 0 12px 0; font-size: 16px; opacity: 0.95; }
        .lede { margin: 0 0 18px 0; font-size: 15px; }
        .cta { display: inline-block; background: #ffffff; color: #0f172a; padding: 12px 18px; border-radius: 10px; text-decoration: none; font-weight: 700; box-shadow: 0 8px 20px rgba(0,0,0,0.12); }
        .card { margin-top: 24px; padding: 18px; border: 1px solid #e5e7eb; border-radius: 12px; background: #f8fafc; }
        .card h3 { margin-top: 0; color: #0f172a; }
        .card p { margin: 6px 0; }
        .card .note { margin-top: 12px; color: #475569; }
        .hero-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 16px; margin-top: 18px; }
        .hero-card { background: #0b766c0d; border: 1px solid #d1fae5; border-radius: 12px; padding: 12px; text-align: center; }
        .hero-label { font-weight: 700; color: #0f766e; margin-bottom: 10px; }
        .hero-card img { max-width: 100%%; border-radius: 10px; }
        .footer { margin-top: 24px; font-size: 13px; color: #6b7280; text-align: left; }
        .footer a { color: #0ea5e9; text-decoration: none; }
    </style>
</head>
<body>
    <div class="preheader">%s</div>
    <div class="container">
        <div class="hero">
            <p class="eyebrow">CleanApp alert</p>
            <h1>Major new issue reported for your brand</h1>
            <p class="subhead">Someone just submitted a brand-related digital report.</p>
            <p class="lede">CleanApp AI analyzed this issue to highlight potential legal and risk ranges connected to your brand presence.</p>
            <a class="cta" href="%s">Open brand dashboard</a>
        </div>

        <div class="card">
            <h3>AI analysis summary</h3>
            <p><strong>Title:</strong> %s</p>
            <p><strong>Description:</strong> %s</p>
            <p><strong>Type:</strong> Digital Issue</p>
            <p class="note">Review the dashboard to see the AI rationale, mapped legal/risk ranges, and supporting media.</p>
        </div>%s

        <div class="footer">
            <p>To unsubscribe from these emails, please <a href="%s?email=%s">click here</a>.</p>
        </div>
    </div>
</body>
</html>`,
			subjectLine,
			preheader,
			e.config.BrandDashboardURL,
			analysis.Title,
			analysis.Description,
			heroImages,
			e.config.OptOutURL,
			recipient)
	}

	// Calculate gauge colors based on values
	litterColor := e.getGaugeColor(analysis.LitterProbability)
	hazardColor := e.getGaugeColor(analysis.HazardProbability)
	severityColor := e.getSeverityGaugeColor(analysis.SeverityLevel)

	imagesSection := ""
	if hasReport {
		imagesSection += fmt.Sprintf(`
        <div class="image-container">
            <h3>Report Image:</h3>
            <img src="cid:%s" alt="Report Image" style="max-width: 100%%; height: auto; border-radius: 5px;">
        </div>`, reportImgCid)
	}
	if hasMap {
		imagesSection += fmt.Sprintf(`
        <div class="image-container">
            <h3>Location Map:</h3>
            <img src="cid:%s" alt="Map" style="max-width: 100%%; height: auto; border-radius: 5px;">
        </div>`, mapImgCid)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>CleanApp Report: %s</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .header { background-color: #f8f9fa; padding: 20px; border-radius: 5px; margin-bottom: 20px; }
        .analysis-section { background-color: #e9ecef; padding: 15px; border-radius: 5px; margin: 15px 0; }
        .gauge-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 15px; margin: 20px 0; }
        .gauge-item { background-color: #fff; padding: 15px; border-radius: 8px; text-align: center; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .gauge-title { font-size: 0.9em; font-weight: bold; margin-bottom: 10px; color: #555; }
        .gauge-container { position: relative; width: 100%%; height: 60px; background: #f0f0f0; border-radius: 30px; overflow: hidden; margin: 10px 0; }
        .gauge-fill { height: 100%%; border-radius: 30px; transition: width 0.3s ease; position: relative; }
        .gauge-fill::after { content: ''; position: absolute; top: 2px; right: 2px; width: 8px; height: calc(100%% - 4px); background: rgba(255,255,255,0.3); border-radius: 4px; }
        .gauge-value { font-size: 1.3em; font-weight: bold; margin-top: 8px; }
        .gauge-label { font-size: 0.8em; color: #666; margin-top: 5px; }
        .images { margin: 20px 0; }
        .image-container { margin: 15px 0; }
        .low { background: linear-gradient(90deg, #28a745, #20c997); }
        .medium { background: linear-gradient(90deg, #ffc107, #fd7e14); }
        .high { background: linear-gradient(90deg, #dc3545, #e83e8c); }
        .digital-notice { background-color: #fff3cd; padding: 15px; border-radius: 5px; margin: 15px 0; border-left: 4px solid #ffc107; }
    </style>
</head>
<body>
    <div class="header">
        <h2>CleanApp Report Analysis</h2>
        <p>A new report has been analyzed and requires your attention.</p>
    </div>

    <div class="analysis-section">
        <h3>Report Details</h3>
        <p><strong>Title:</strong> %s</p>
        <p><strong>Description:</strong> %s</p>
        <p><strong>Type:</strong> %s</p>
    </div>

    %s

    <div class="images">%s
    </div>

    <p><em>Best regards,<br>The CleanApp Team</em></p>

    <div style="margin-top: 30px; padding-top: 20px; border-top: 1px solid #eee; font-size: 0.9em; color: #666;">
        <p>To unsubscribe from these emails, please <a href="%s?email=%s" style="color: #007bff; text-decoration: none;">click here</a></p>
    </div>
</body>
</html>`,
		analysis.Title,
		analysis.Title,
		analysis.Description,
		analysis.Classification,
		e.getMetricsSection(analysis, isDigital, litterColor, hazardColor, severityColor),
		imagesSection,
		e.config.OptOutURL,
		recipient)
}

// getMetricsSection returns the appropriate metrics section based on report type
func (e *EmailSender) getMetricsSection(analysis *models.ReportAnalysis, isDigital bool, litterColor, hazardColor, severityColor string) string {
	if isDigital {
		// For digital reports, show a notice instead of metrics
		return ""
	}

	// For physical reports, show the metrics gauge
	return fmt.Sprintf(`
    <div class="gauge-grid">
        <div class="gauge-item">
            <div class="gauge-title">Litter Probability</div>
            <div class="gauge-container">
                <div class="gauge-fill %s" style="width: %.1f%%;"></div>
            </div>
            <div class="gauge-value">%.1f%%</div>
            <div class="gauge-label">%s</div>
        </div>
        
        <div class="gauge-item">
            <div class="gauge-title">Hazard Probability</div>
            <div class="gauge-container">
                <div class="gauge-fill %s" style="width: %.1f%%;"></div>
            </div>
            <div class="gauge-value">%.1f%%</div>
            <div class="gauge-label">%s</div>
        </div>
        
        <div class="gauge-item">
            <div class="gauge-title">Severity Level</div>
            <div class="gauge-container">
                <div class="gauge-fill %s" style="width: %.1f%%;"></div>
            </div>
            <div class="gauge-value">%.1f</div>
            <div class="gauge-label">%s</div>
        </div>
    </div>`,
		litterColor, analysis.LitterProbability*100, analysis.LitterProbability*100, e.getGaugeLabel(analysis.LitterProbability),
		hazardColor, analysis.HazardProbability*100, analysis.HazardProbability*100, e.getGaugeLabel(analysis.HazardProbability),
		severityColor, analysis.SeverityLevel*10, analysis.SeverityLevel*10, e.getSeverityGaugeLabel(analysis.SeverityLevel))
}

// getGaugeColor returns the CSS class for gauge color based on value
func (e *EmailSender) getGaugeColor(value float64) string {
	if value < 0.3 {
		return "low"
	} else if value < 0.7 {
		return "medium"
	} else {
		return "high"
	}
}

// getGaugeLabel returns a descriptive label based on the value
func (e *EmailSender) getGaugeLabel(value float64) string {
	if value < 0.3 {
		return "Low"
	} else if value < 0.7 {
		return "Medium"
	} else {
		return "High"
	}
}

// getSeverityGaugeColor returns the CSS class for severity gauge color based on 0-10 scale
func (e *EmailSender) getSeverityGaugeColor(value float64) string {
	if value < 3.0 {
		return "low"
	} else if value < 7.0 {
		return "medium"
	} else {
		return "high"
	}
}

// getSeverityGaugeLabel returns a descriptive label for severity based on 0-10 scale
func (e *EmailSender) getSeverityGaugeLabel(value float64) string {
	if value < 3.0 {
		return "Low"
	} else if value < 7.0 {
		return "Medium"
	} else {
		return "High"
	}
}
