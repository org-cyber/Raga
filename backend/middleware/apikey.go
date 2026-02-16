package middleware

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		/*Gin middleware must return a function of type gin.HandlerFunc.
		Think of middleware as:
		A security checkpoint before the route executes*/

		//get api key from request header
		apikey := c.GetHeader("x-api-key")
		fmt.Println("postman sent:", apikey)
		//get the expected api key from the env variable
		expectedKey := os.Getenv("ASGUARD_API_KEY")
		fmt.Println("the program is looking for", expectedKey)

		//error handeling to check if the api key is corect or not even there
		if apikey == "" || apikey != expectedKey {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorised",
			})
			c.Abort()
			return
		}

		//if valid continue
		c.Next()
	}
}
