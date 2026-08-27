# LinkedIn Profile API (Go)

A fast, concurrent, and robust Go-based API for fetching and normalizing public LinkedIn profiles. 

Give it a LinkedIn member URL, and it returns the profile as a structured, predictable JSON response. It uses the authenticated, read-only HTTP endpoints used by LinkedIn's web client, running blazingly fast in a Go environment.

## Features
- **Written in Go**: Highly concurrent and memory-efficient.
- **In-Memory Caching**: Thread-safe LRU cache to prevent hammering upstream endpoints.
- **Singleflight Concurrency**: Prevents redundant upstream calls if multiple identical requests hit the server simultaneously.
- **Rate Limiting**: Built-in IP-based rate limiter to protect the session cookies.
- **Dockerized**: Deploy anywhere easily with zero dependencies.

## Usage

Interactive documentation and health endpoints are available out-of-the-box.

```bash
curl --request POST 'http://localhost:8080/v1/profiles' \
  --header 'content-type: application/json' \
  --data '{"url":"https://www.linkedin.com/in/vineetmotwani/"}'
```

### Safety & Anti-Bot Precautions
This API requires LinkedIn session cookies (`li_at` and `JSESSIONID`).
**Never use your personal LinkedIn account cookies**. Always create a separate, low-privilege dummy account to extract the cookies. Rapidly scraping hundreds of unique profiles will trigger LinkedIn's anti-bot protections and could result in the account being banned.

## Local Development & Testing

You just need Go 1.21+ installed, or Docker.

### 1. Set your Environment Variables
Create a `.env` file in the root directory:
```env
LINKEDIN_LI_AT=your_dummy_cookie
LINKEDIN_JSESSIONID=ajax:your_dummy_csrf
```

### 2. Run Locally
```bash
go build -o linkedin-api ./main.go
./linkedin-api
```

### 3. Run with Docker
```bash
docker build -t linkedin-api .
docker run -p 8080:8080 --env-file .env linkedin-api
```

## Free Deployment (Koyeb / Render)

Because this project includes a standard `Dockerfile`, it can be deployed on any modern SaaS platform for free with zero lock-in:

1. Push your repository to GitHub.
2. Sign up for [Koyeb](https://app.koyeb.com/) or Render.
3. Click **Create Web Service** -> **GitHub** and select your repository.
4. In the deployment settings, add the `LINKEDIN_LI_AT` and `LINKEDIN_JSESSIONID` Environment Variables.
5. Deploy!
