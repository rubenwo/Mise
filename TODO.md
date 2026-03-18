# issues and improvements

1. Recipe generation seems to fail more often now that we have a few recipes. Possibly an issue with a large context in our generate calls. Let's try to limit the size of our context and do as much processing through conventional code where possible. For example de-duplication could be handled better if we do some of this through code instead of making the small LLM figure this out.

2. The edamam API returns a 401, possibly some incorrect authentication from our side. Review this.

3. The review prompt often takes a long time. Possibly check if another model is better suited for this than the initial generator model.

4. Feature request: add background recipe generation based on a task that runs every x amount of time. The next time the site opens a list of recipes should be presented to go through. The interval should be configurable.

5. Feature improvement: when a ollama backend suddenly is no longer available, keep it in the pool, but don't send chat completion calls to it. Do a keepalive (maybe the /api/tags endpoint) to check if/when it's back. We're running on consumer/gaming hardware. Sometimes I'm using my cards to game and don't want it to be used by Ollama.

6. Feature request: I have multiple different types of Ollama backends. My main gaming PC contains an RTX 3080 (10GB). Very fast inference on models smaller than 9b parameters. I have an Nvidia P40 (24GB) in my server. About 3x slower, but can run a 27b parameter model for better reasoning. For background tasks I would prefer to use my slower (always online) card. To keep this a bit generic we could add a tag to the ollama provider setting and make it configurable in the frontend.

7. The deduplication of ingredients in our plan is currently slow and unreliable. It really depends on the model what the quality of the normalization is and if it even returns in time.