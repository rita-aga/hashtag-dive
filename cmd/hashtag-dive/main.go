package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func main() {
	// opts := []chromedp.ExecAllocatorOption{
	// 	chromedp.ExecPath(`/Applications/Chromium.app/Contents/MacOS/Chromium`),
	// 	chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/77.0.3830.0 Safari/537.36"),
	// 	chromedp.WindowSize(1920, 1080),
	// 	chromedp.NoFirstRun,
	// 	chromedp.NoDefaultBrowserCheck,
	// 	chromedp.Headless,
	// 	chromedp.DisableGPU,
	// }
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/77.0.3830.0 Safari/537.36"),
		chromedp.WindowSize(1920, 1080),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// also set up a custom logger
	taskCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	// see: https://intoli.com/blog/not-possible-to-block-chrome-headless/
	const script = `(function(w, n, wn) {
			// Pass the Webdriver Test.
			Object.defineProperty(n, 'webdriver', {
			  get: () => false,
			});
		  
			// Pass the Plugins Length Test.
			// Overwrite the plugins property to use a custom getter.
			Object.defineProperty(n, 'plugins', {
			  // This just needs to have length > 0 for the current test,
			  // but we could mock the plugins too if necessary.
			  get: () => [1, 2, 3, 4, 5],
			});
		  
			// Pass the Languages Test.
			// Overwrite the plugins property to use a custom getter.
			Object.defineProperty(n, 'languages', {
			  get: () => ['en-US', 'en'],
			});
		  
			// Pass the Chrome Test.
			// We can mock this in as much depth as we need for the test.
			w.chrome = {
			  runtime: {},
			};
		  
			// Pass the Permissions Test.
			const originalQuery = wn.permissions.query;
			return wn.permissions.query = (parameters) => (
			  parameters.name === 'notifications' ?
				Promise.resolve({ state: Notification.permission }) :
				originalQuery(parameters)
			);
		  
		  })(window, navigator, window.navigator);`

	// define hashtag
	hashtag := "35mm"

	// number of posts to process
	// postNum := 50

	// run task list
	var hashtagPosts []*cdp.Node

	var postOwners []string
	// postOwners = []

	err := chromedp.Run(taskCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		_, err = page.AddScriptToEvaluateOnNewDocument(script).Do(ctx)
		if err != nil {
			return err
		}
		return nil
	}),
		login(),
		goToHashtag(hashtag),
		getPosts(&hashtagPosts))

	if err != nil {
		log.Fatal(err)
	}

	for _, p := range hashtagPosts {
		user, err := getPostOwner(taskCtx, p)
		if err != nil {
			log.Fatal(err)
		} else {
			postOwners = append(postOwners, user)
		}
	}

	log.Printf("got `%v` users", len(postOwners))

	for _, u := range postOwners {
		err := processUser(taskCtx, u)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func addSleep(actionTasks chromedp.Tasks) chromedp.Tasks {
	tasks := chromedp.Tasks{}
	for _, t := range actionTasks {
		tasks = append(tasks, t)
		tasks = append(tasks, chromedp.Sleep(3*time.Second))
	}
	return tasks
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func itemExists(slice interface{}, item interface{}) bool {
	s := reflect.ValueOf(slice)

	if s.Kind() != reflect.Slice {
		panic("Invalid data-type")
	}

	for i := 0; i < s.Len(); i++ {
		if s.Index(i).Interface() == item {
			return true
		}
	}

	return false
}

func login() chromedp.Tasks {
	actionTasks := chromedp.Tasks{
		chromedp.Navigate("https://www.instagram.com/accounts/login/"),
		chromedp.WaitVisible(`//*[@name="username"]`),
		chromedp.SendKeys(`//*[@name="username"]`, ""),
		chromedp.SendKeys(`//*[@name="password"]`, ""),
		chromedp.WaitEnabled(`//button[@type="submit"]`),
		chromedp.Click(`//button[@type="submit"]`),
		chromedp.WaitVisible(`//button[@type="button"]`),
	}

	return addSleep(actionTasks)
}

func goToHashtag(hashtag string) chromedp.Tasks {
	url := fmt.Sprintf("https://www.instagram.com/explore/tags/%v/", hashtag)
	actionTasks := chromedp.Tasks{
		chromedp.Navigate(url),
		chromedp.WaitVisible(`//*[@class="v1Nh3 kIKUG  _bz0w"]`),
	}

	return addSleep(actionTasks)
}

func getPosts(res *[]*cdp.Node) chromedp.Tasks {
	actionTasks := chromedp.Tasks{
		// TODO check if elements present
		chromedp.Nodes(`//*[@class="v1Nh3 kIKUG  _bz0w"]`, res),
	}

	return addSleep(actionTasks)
}

func getPostOwner(ctx context.Context, post *cdp.Node) (string, error) {
	var user string
	// click on post
	if err := chromedp.Run(ctx, chromedp.MouseClickNode(post)); err != nil {
		return "", fmt.Errorf("Could not click on post: %v", err)
	}
	log.Printf("clicked on post")
	time.Sleep(2 * time.Second)

	// wait for link to user
	if err := chromedp.Run(ctx, chromedp.WaitVisible(`//*[@class="sqdOP yWX7d     _8A5w5   ZIAjV "]`)); err != nil {
		return "", fmt.Errorf("Could not locate link to user: %v", err)
	}
	log.Printf("found user ele")
	time.Sleep(2 * time.Second)

	// get user name
	if err := chromedp.Run(ctx, chromedp.Text(`//*[@class="sqdOP yWX7d     _8A5w5   ZIAjV "]`, &user)); err != nil {
		return "", fmt.Errorf("Could not locate link to user: %v", err)
	}
	log.Printf("got username")
	time.Sleep(2 * time.Second)

	// wait for like btn
	if err := chromedp.Run(ctx, chromedp.WaitVisible(`//span[@class="fr66n"]`)); err != nil {
		return "", fmt.Errorf("Could not locate like button: %v", err)
	}
	log.Printf("found like btn")
	time.Sleep(2 * time.Second)

	// click on like btn
	if err := chromedp.Run(ctx, chromedp.Click(`//span[@class="fr66n"]`)); err != nil {
		return "", fmt.Errorf("Could not click like button: %v", err)
	}
	log.Printf("clicked like btn")
	time.Sleep(2 * time.Second)

	// close post modal
	if err := chromedp.Run(ctx, chromedp.MouseClickXY(0.05, 0.05)); err != nil {
		return "", fmt.Errorf("Could not close post modal: %v", err)
	}
	log.Printf("closed modal")
	time.Sleep(2 * time.Second)

	return user, nil
}

func processUser(ctx context.Context, user string) error {
	var file *os.File
	var userPosts []*cdp.Node
	// check if file with proccessed users exists
	if fileExists("processed_users.txt") {
		fmt.Println("processed_users.txt exists")
	} else {
		fmt.Println("processed_users.txt does not exist, creating...")
		// create file if doesn't exist
		f, err := os.Create("processed_users.txt")
		if err != nil {
			return fmt.Errorf("Error creating processed_users.txt: %v", err)
		}
		file = f
	}

	// check in file if user is processed
	dat, err := ioutil.ReadFile("processed_users.txt")
	if err != nil {
		return fmt.Errorf("Error reading processed_users.txt: %v", err)
	}
	usersStr := string(dat)
	processedUsers := strings.Split(usersStr, "\n")
	if itemExists(processedUsers, user) {
		return nil
	}

	// go to user page
	if err := chromedp.Run(ctx, chromedp.Navigate(fmt.Sprintf("https://www.instagram.com/%v/", user))); err != nil {
		return fmt.Errorf("Could not navigate to user page: %v", err)
	}
	log.Printf("navigated to user page")
	time.Sleep(3 * time.Second)

	// get post elements
	if err := chromedp.Run(ctx, getPosts(&userPosts)); err != nil {
		return fmt.Errorf("Could not get posts: %v", err)
	}
	log.Printf("got posts")
	time.Sleep(3 * time.Second)

	// loop through post elements and process them
	for i, p := range userPosts {
		if i > 10 {
			// mark user as proccessed in file
			// TODO fix
			n, _ := file.WriteString(fmt.Sprintf("%v\n", user))
			fmt.Printf("wrote %d bytes\n", n)
			return nil
		}
		err := processPost(ctx, p)
		if err != nil {
			log.Fatal(err)
		}
	}

	// mark user as proccessed in file
	// TODO fix
	n, err := file.WriteString(fmt.Sprintf("%v\n", user))
	fmt.Printf("wrote %d bytes\n", n)
	return nil
}

func processPost(ctx context.Context, post *cdp.Node) error {
	// click on post
	if err := chromedp.Run(ctx, chromedp.MouseClickNode(post)); err != nil {
		return fmt.Errorf("Could not click on post: %v", err)
	}
	log.Printf("clicked on post")
	time.Sleep(3 * time.Second)

	// wait for like btn
	if err := chromedp.Run(ctx, chromedp.WaitVisible(`//span[@class="fr66n"]`)); err != nil {
		return fmt.Errorf("Could not locate like button: %v", err)
	}
	log.Printf("found like btn")
	time.Sleep(3 * time.Second)

	// click on like btn
	if err := chromedp.Run(ctx, chromedp.Click(`//span[@class="fr66n"]`)); err != nil {
		return fmt.Errorf("Could not click like button: %v", err)
	}
	log.Printf("clicked like btn")
	time.Sleep(3 * time.Second)

	// close modal
	if err := chromedp.Run(ctx, chromedp.MouseClickXY(0.05, 0.05)); err != nil {
		return fmt.Errorf("Could not close post modal: %v", err)
	}
	log.Printf("closed modal")
	time.Sleep(3 * time.Second)

	return nil
}
