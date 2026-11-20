// reset.js: a form with data-reset-on-success empties itself once its htmx
// request succeeded. A response with HX-Redirect (an error shown on the
// next page) leaves the form as the user typed it.
document.body.addEventListener("htmx:afterRequest", function (evt) {
  var form = evt.detail.elt;
  if (!form || !form.matches || !form.matches("form[data-reset-on-success]")) return;
  var xhr = evt.detail.xhr;
  if (evt.detail.successful && !(xhr && xhr.getResponseHeader("HX-Redirect"))) form.reset();
});
