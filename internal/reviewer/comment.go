package reviewer

// // processCommentEvent handles comment related events
// func (s *Reviewer) processCommentEvent(ctx context.Context, event *model.CodeEvent, log logze.Logger) error {
// 	if event.Comment == nil || event.MergeRequest == nil {
// 		return errm.New("comment or merge request is nil in event")
// 	}

// 	log = log.WithFields(
// 		"comment_id", event.Comment.ID,
// 		"mr_iid", event.MergeRequest.IID,
// 		"comment_author", event.Comment.Author.Username,
// 	)

// 	// Check if this is a reply to our bot's comment
// 	if !s.isReplyToBotComment(ctx, event) {
// 		log.Debug("comment event not relevant for processing")
// 		return nil
// 	}

// 	log.Info("processing reply to bot comment")
// 	return s.ProcessCommentReply(ctx, event.ProjectID, event.MergeRequest.IID, event.Comment)
// }

// // ProcessCommentReply handles replies to bot comments
// func (s *Reviewer) ProcessCommentReply(ctx context.Context, request *model.ReviewRequest, comment *model.Comment) error {
// 	log := s.log.WithFields(
// 		"project_id", request.ProjectID,
// 		"mr_iid", request.MergeRequest.IID,
// 		"comment_id", comment.ID,
// 		"comment_author", comment.Author.Username,
// 	)

// 	// Get the original comment being replied to
// 	originalComment, err := s.provider.GetComment(ctx, request.ProjectID, request.MergeRequest.IID, comment.ParentID)
// 	if err != nil {
// 		return errm.Wrap(err, "failed to get original comment")
// 	}

// 	// Generate a contextual reply
// 	replyContext := fmt.Sprintf("Original comment: %s\nUser reply: %s", originalComment.Body, comment.Body)
// 	reply, err := s.generateWithRetry(ctx, func() (string, error) {
// 		return s.agent.GenerateCommentReply(ctx, originalComment.Body, replyContext)
// 	}, log)
// 	if err != nil {
// 		return errm.Wrap(err, "failed to generate reply")
// 	}

// 	if reply == "" {
// 		log.Debug("agent returned empty reply")
// 		return nil
// 	}

// 	// Post the reply
// 	err = s.provider.ReplyToComment(ctx, request.ProjectID, request.MergeRequest.IID, comment.ID, reply)
// 	if err != nil {
// 		return errm.Wrap(err, "failed to post reply")
// 	} else {
// 		s.markFileAsReviewed(request, comment.FilePath, "")
// 		log.Info("successfully posted reply to comment")
// 	}

// 	return nil
// }

// // isReplyToBotComment checks if a comment is a reply to the bot's comment
// func (s *Reviewer) isReplyToBotComment(ctx context.Context, event *model.CodeEvent) bool {

// 	// Get the parent comment
// 	parentComment, err := s.provider.GetComment(ctx, event.ProjectID, event.MergeRequest.IID, event.Comment.ParentID)
// 	if err != nil {
// 		s.log.Err(err, "failed to get parent co	mment for reply check")
// 		return false
// 	}

// 	// Check if parent comment is from our bot
// 	return parentComment.Author.Username == s.config.Provider.BotUsername
// }
