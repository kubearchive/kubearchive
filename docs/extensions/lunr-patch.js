var lunr = require('lunr')

module.exports.register = function() {
  this.on('contextStarted', ({}) => {
    // Disable hyphen separator
    lunr.tokenizer.separator = /\s+/

    // Disable Lunr stemming by overriding
    lunr.stemmer = (function() {
        return function (token) {
            return token;
        }
    })();
    lunr.Pipeline.registerFunction(lunr.stemmer, 'stemmer')

  })
}
